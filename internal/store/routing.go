package store

import "time"

type RoutingEvent struct {
	TS               int64
	RequestID        string
	SessionID        string
	StateText        string
	PredComplexity   string
	PredNeedsTools   bool
	PredConfidence   float64
	ClassifierSource string
	ChosenModel      string
	CostTier         string
	Reason           string
	Escalated        bool
	StickyBreak      bool
	UserOverride     bool
	LocalSuccess     bool
}

type QualitySummary struct {
	Total         int64   `json:"total"`
	LocalSharePct float64 `json:"local_share_pct"`
	Escalations   int64   `json:"escalations"`
	Overrides     int64   `json:"overrides"`
	StickyBreaks  int64   `json:"sticky_breaks"`
}

func (d *DB) InsertRouting(e RoutingEvent) error {
	if e.TS == 0 {
		e.TS = time.Now().UnixMilli()
	}
	b := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	_, err := d.SQL.Exec(`
INSERT INTO routing_events(ts, request_id, session_id, state_text, pred_complexity, pred_needs_tools,
  pred_confidence, classifier_source, chosen_model, cost_tier, reason, escalated, sticky_break, user_override, local_success)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.TS, e.RequestID, e.SessionID, truncate(e.StateText, 4000), e.PredComplexity, b(e.PredNeedsTools),
		e.PredConfidence, e.ClassifierSource, e.ChosenModel, e.CostTier, e.Reason,
		b(e.Escalated), b(e.StickyBreak), b(e.UserOverride), b(e.LocalSuccess))
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func (d *DB) QualitySince(sinceMS int64) (QualitySummary, error) {
	var q QualitySummary
	err := d.SQL.QueryRow(`SELECT COUNT(*) FROM routing_events WHERE ts>=?`, sinceMS).Scan(&q.Total)
	if err != nil {
		return q, err
	}
	if q.Total == 0 {
		return q, nil
	}
	var local int64
	_ = d.SQL.QueryRow(`SELECT COUNT(*) FROM routing_events WHERE ts>=? AND cost_tier='local'`, sinceMS).Scan(&local)
	_ = d.SQL.QueryRow(`SELECT COUNT(*) FROM routing_events WHERE ts>=? AND escalated=1`, sinceMS).Scan(&q.Escalations)
	_ = d.SQL.QueryRow(`SELECT COUNT(*) FROM routing_events WHERE ts>=? AND user_override=1`, sinceMS).Scan(&q.Overrides)
	_ = d.SQL.QueryRow(`SELECT COUNT(*) FROM routing_events WHERE ts>=? AND sticky_break=1`, sinceMS).Scan(&q.StickyBreaks)
	q.LocalSharePct = float64(local) * 100 / float64(q.Total)
	return q, nil
}
