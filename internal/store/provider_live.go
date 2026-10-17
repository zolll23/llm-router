package store

import (
	"sort"
	"time"
)

type ProviderBucket struct {
	TS  int64 `json:"ts"`
	Req int   `json:"req"`
}

type ProviderLive struct {
	ProviderID string           `json:"provider_id"`
	Label      string           `json:"label"`
	Series     []ProviderBucket `json:"series"`
	Latency    LatencyStats     `json:"latency"`
}

// ProviderLiveSince returns per-provider request buckets and latency percentiles
// for events with ts >= sinceMS. bucketMS controls series granularity (e.g. 60_000).
func (d *DB) ProviderLiveSince(sinceMS, bucketMS int64) ([]ProviderLive, error) {
	if bucketMS <= 0 {
		bucketMS = 60_000
	}
	rows, err := d.SQL.Query(`
SELECT provider_id, (ts/?)*? AS bucket, COUNT(*)
FROM usage_events
WHERE ts >= ? AND provider_id IS NOT NULL AND TRIM(provider_id) != ''
GROUP BY provider_id, bucket
ORDER BY provider_id, bucket`, bucketMS, bucketMS, sinceMS)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byProv := map[string]*ProviderLive{}
	var order []string
	for rows.Next() {
		var pid string
		var bucket int64
		var n int
		if err := rows.Scan(&pid, &bucket, &n); err != nil {
			return nil, err
		}
		p := byProv[pid]
		if p == nil {
			p = &ProviderLive{ProviderID: pid, Label: providerLabel(pid), Series: nil}
			byProv[pid] = p
			order = append(order, pid)
		}
		p.Series = append(p.Series, ProviderBucket{TS: bucket, Req: n})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, pid := range order {
		latRows, err := d.SQL.Query(`
SELECT latency_ms FROM usage_events
WHERE ts >= ? AND provider_id = ? AND latency_ms > 0
ORDER BY latency_ms`, sinceMS, pid)
		if err != nil {
			return nil, err
		}
		var vals []float64
		for latRows.Next() {
			var v float64
			if err := latRows.Scan(&v); err != nil {
				latRows.Close()
				return nil, err
			}
			vals = append(vals, v)
		}
		latRows.Close()
		byProv[pid].Latency = percentileStats(vals)
	}

	out := make([]ProviderLive, 0, len(order))
	for _, pid := range order {
		out = append(out, *byProv[pid])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProviderID < out[j].ProviderID })
	return out, nil
}

func providerLabel(id string) string {
	switch id {
	case "ollama":
		return "Local Ollama"
	case "ollama-cloud":
		return "Ollama Cloud"
	case "opencode-go", "opencode":
		return "OpenCode"
	default:
		return id
	}
}

// DefaultLiveWindow is the UI window for live provider charts.
func DefaultLiveWindow() (sinceMS, bucketMS int64) {
	now := time.Now()
	return now.Add(-15 * time.Minute).UnixMilli(), 60_000
}
