package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"router/internal/store"
)

type JudgeConfig struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

func (j JudgeConfig) client() *http.Client {
	if j.Client != nil {
		return j.Client
	}
	return &http.Client{Timeout: 45 * time.Second}
}

type judgeResp struct {
	Answers map[string]struct {
		Choice     string  `json:"choice"`
		Noul       float64 `json:"noul"`
		Confidence float64 `json:"confidence"`
	} `json:"answers"`
}

// RunOnce judges up to limit unlabeled routing events prioritized by escalated/low confidence.
func RunOnce(ctx context.Context, db *store.DB, cfg JudgeConfig, limit int) (int, error) {
	if limit <= 0 {
		limit = 20
	}
	if cfg.Model == "" {
		cfg.Model = "nimble"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://127.0.0.1:11434"
	}
	rows, err := db.SQL.Query(`
SELECT r.request_id, COALESCE(r.state_text,''), COALESCE(r.pred_complexity,''), COALESCE(r.chosen_model,''), COALESCE(r.cost_tier,''),
       COALESCE(r.escalated,0), COALESCE(r.pred_confidence,0)
FROM routing_events r
LEFT JOIN judge_labels j ON j.request_id = r.request_id
WHERE j.request_id IS NULL AND r.request_id IS NOT NULL AND r.request_id != ''
ORDER BY r.escalated DESC, r.pred_confidence ASC, r.id DESC
LIMIT ?`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		var reqID, state, pred, chosen, tier string
		var escalated int
		var conf float64
		if err := rows.Scan(&reqID, &state, &pred, &chosen, &tier, &escalated, &conf); err != nil {
			return n, err
		}
		gold, under, over, jconf, err := cfg.judge(ctx, state, pred, chosen, tier, escalated == 1)
		if err != nil {
			continue
		}
		_, err = db.SQL.Exec(`
INSERT OR REPLACE INTO judge_labels(request_id, gold_complexity, underkill, overkill, judge_confidence, judge_model, judged_at)
VALUES(?,?,?,?,?,?,?)`, reqID, gold, boolInt(under), boolInt(over), jconf, cfg.Model, time.Now().UnixMilli())
		if err != nil {
			return n, err
		}
		n++
	}
	return n, rows.Err()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (cfg JudgeConfig) judge(ctx context.Context, state, pred, chosen, tier string, escalated bool) (gold string, under, over bool, conf float64, err error) {
	body := map[string]any{
		"model": cfg.Model,
		"state": map[string]any{
			"user_text": state, "pred_complexity": pred, "chosen_model": chosen,
			"cost_tier": tier, "escalated": escalated,
		},
		"questions": map[string]any{
			"gold_complexity": map[string]any{
				"type": "choice", "instructions": "True complexity of the user request",
				"criteria": map[string]string{
					"trivial": "Trivial", "simple": "Simple", "medium": "Medium", "hard": "Hard",
				},
			},
			"underkill": map[string]any{"type": "noul", "instructions": "Was the chosen model too weak?"},
			"overkill":  map[string]any{"type": "noul", "instructions": "Was the chosen model unnecessarily expensive/strong?"},
		},
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.BaseURL, "/")+"/v1/systemone", bytes.NewReader(b))
	if err != nil {
		return "", false, false, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := cfg.client().Do(req)
	if err != nil {
		return "", false, false, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", false, false, 0, fmt.Errorf("judge status %s: %s", resp.Status, string(raw))
	}
	var parsed judgeResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", false, false, 0, err
	}
	g := parsed.Answers["gold_complexity"].Choice
	if g == "" {
		g = pred
	}
	return g, parsed.Answers["underkill"].Noul >= 0.5, parsed.Answers["overkill"].Noul >= 0.5, parsed.Answers["gold_complexity"].Confidence, nil
}

type Confusion struct {
	Labels []string         `json:"labels"`
	Matrix [][]int          `json:"matrix"`
	Counts map[string]int   `json:"counts"`
}

func ConfusionMatrix(db *store.DB, sinceMS int64) (Confusion, error) {
	labels := []string{"trivial", "simple", "medium", "hard"}
	idx := map[string]int{}
	for i, l := range labels {
		idx[l] = i
	}
	m := make([][]int, len(labels))
	for i := range m {
		m[i] = make([]int, len(labels))
	}
	rows, err := db.SQL.Query(`
SELECT r.pred_complexity, j.gold_complexity
FROM routing_events r
JOIN judge_labels j ON j.request_id = r.request_id
WHERE r.ts >= ?`, sinceMS)
	if err != nil {
		return Confusion{}, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var pred, gold string
		if err := rows.Scan(&pred, &gold); err != nil {
			return Confusion{}, err
		}
		pi, ok1 := idx[pred]
		gi, ok2 := idx[gold]
		if !ok1 || !ok2 {
			continue
		}
		m[pi][gi]++
		counts[pred+"->"+gold]++
	}
	return Confusion{Labels: labels, Matrix: m, Counts: counts}, rows.Err()
}

func StartScheduler(ctx context.Context, db *store.DB, cfg JudgeConfig, every time.Duration) {
	if every <= 0 {
		every = 6 * time.Hour
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_, _ = RunOnce(ctx, db, cfg, 25)
			}
		}
	}()
}
