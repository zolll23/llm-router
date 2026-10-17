package store

import (
	"time"
)

type UsageEvent struct {
	TS                int64
	ModelID           string
	ProviderID        string
	PromptTokens      int
	CachedTokens      int
	CompletionTokens  int
	LatencyMS         int
	SessionID         string
	Complexity        string
	Confidence        float64
	ClassifierSource  string
	HTTPStatus        int
	Estimated         bool
}

type ModelWindowStats struct {
	ModelID          string  `json:"model_id"`
	ProviderID       string  `json:"provider_id"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CachedTokens     int64   `json:"cached_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	Requests         int64   `json:"requests"`
	SharePct         float64 `json:"share_pct"`
}

type Overview struct {
	Day   []ModelWindowStats `json:"day"`
	Week  []ModelWindowStats `json:"week"`
	Month []ModelWindowStats `json:"month"`
}

func (d *DB) InsertUsage(e UsageEvent) error {
	if e.TS == 0 {
		e.TS = time.Now().UnixMilli()
	}
	est := 0
	if e.Estimated {
		est = 1
	}
	_, err := d.SQL.Exec(`
INSERT INTO usage_events(ts, model_id, provider_id, prompt_tokens, cached_tokens, completion_tokens,
  latency_ms, session_id, complexity, confidence, classifier_source, http_status, estimated)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.TS, e.ModelID, e.ProviderID, e.PromptTokens, e.CachedTokens, e.CompletionTokens,
		e.LatencyMS, e.SessionID, e.Complexity, e.Confidence, e.ClassifierSource, e.HTTPStatus, est)
	return err
}

func (d *DB) aggregateSince(sinceMS int64) ([]ModelWindowStats, error) {
	rows, err := d.SQL.Query(`
SELECT model_id, provider_id,
  COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(cached_tokens),0), COALESCE(SUM(completion_tokens),0),
  COUNT(*)
FROM usage_events WHERE ts >= ?
GROUP BY model_id, provider_id
ORDER BY COUNT(*) DESC`, sinceMS)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelWindowStats
	var total int64
	for rows.Next() {
		var s ModelWindowStats
		if err := rows.Scan(&s.ModelID, &s.ProviderID, &s.PromptTokens, &s.CachedTokens, &s.CompletionTokens, &s.Requests); err != nil {
			return nil, err
		}
		total += s.Requests
		out = append(out, s)
	}
	if total > 0 {
		for i := range out {
			out[i].SharePct = float64(out[i].Requests) * 100 / float64(total)
		}
	}
	return out, rows.Err()
}

func (d *DB) Overview(now time.Time) (Overview, error) {
	dayAgo := now.Add(-24 * time.Hour).UnixMilli()
	weekAgo := now.Add(-7 * 24 * time.Hour).UnixMilli()
	monthAgo := now.Add(-30 * 24 * time.Hour).UnixMilli()
	var o Overview
	var err error
	if o.Day, err = d.aggregateSince(dayAgo); err != nil {
		return o, err
	}
	if o.Week, err = d.aggregateSince(weekAgo); err != nil {
		return o, err
	}
	if o.Month, err = d.aggregateSince(monthAgo); err != nil {
		return o, err
	}
	return o, nil
}

type LatencyStats struct {
	P50 float64 `json:"p50"`
	P75 float64 `json:"p75"`
	P90 float64 `json:"p90"`
	P95 float64 `json:"p95"`
	N   int     `json:"n"`
}

func (d *DB) ModelDetail(modelID string, sinceMS int64) (LatencyStats, []struct {
	TS  int64 `json:"ts"`
	Req int   `json:"req"`
}, error) {
	rows, err := d.SQL.Query(`SELECT latency_ms FROM usage_events WHERE model_id=? AND ts>=? AND latency_ms>0 ORDER BY latency_ms`, modelID, sinceMS)
	if err != nil {
		return LatencyStats{}, nil, err
	}
	var vals []float64
	for rows.Next() {
		var v float64
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return LatencyStats{}, nil, err
		}
		vals = append(vals, v)
	}
	rows.Close()
	lat := percentileStats(vals)

	bRows, err := d.SQL.Query(`
SELECT (ts/3600000)*3600000 AS bucket, COUNT(*)
FROM usage_events WHERE model_id=? AND ts>=?
GROUP BY bucket ORDER BY bucket`, modelID, sinceMS)
	if err != nil {
		return lat, nil, err
	}
	defer bRows.Close()
	var series []struct {
		TS  int64 `json:"ts"`
		Req int   `json:"req"`
	}
	for bRows.Next() {
		var ts int64
		var n int
		if err := bRows.Scan(&ts, &n); err != nil {
			return lat, nil, err
		}
		series = append(series, struct {
			TS  int64 `json:"ts"`
			Req int   `json:"req"`
		}{TS: ts, Req: n})
	}
	return lat, series, bRows.Err()
}

func percentileStats(sorted []float64) LatencyStats {
	n := len(sorted)
	if n == 0 {
		return LatencyStats{}
	}
	pct := func(p float64) float64 {
		idx := int(float64(n-1) * p)
		return sorted[idx]
	}
	return LatencyStats{P50: pct(0.50), P75: pct(0.75), P90: pct(0.90), P95: pct(0.95), N: n}
}
