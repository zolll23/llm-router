package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"router/internal/store"
)

type FinetuneRow struct {
	Input  string `json:"input"`
	Output struct {
		Classifications []struct {
			Task      string   `json:"task"`
			Labels    []string `json:"labels"`
			TrueLabel []string `json:"true_label"`
		} `json:"classifications"`
	} `json:"output"`
}

// ExportGLiNERJSONL writes judged (or heuristic-proxy) rows for complexity fine-tune.
// Until judge_labels are populated, exports routing_events with pred as weak labels tagged source=pred.
func ExportGLiNERJSONL(db *store.DB, dataDir string, limit int) (string, int, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := db.SQL.Query(`
SELECT COALESCE(state_text,''), COALESCE(pred_complexity,'simple')
FROM routing_events
WHERE pred_complexity IS NOT NULL AND pred_complexity != ''
ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return "", 0, err
	}
	defer rows.Close()

	outDir := filepath.Join(dataDir, "finetune")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", 0, err
	}
	path := filepath.Join(outDir, time.Now().Format("20060102_150405")+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	labels := []string{"trivial", "simple", "medium", "hard"}
	n := 0
	for rows.Next() {
		var input, complexity string
		if err := rows.Scan(&input, &complexity); err != nil {
			return "", 0, err
		}
		if input == "" {
			continue
		}
		row := FinetuneRow{Input: input}
		row.Output.Classifications = []struct {
			Task      string   `json:"task"`
			Labels    []string `json:"labels"`
			TrueLabel []string `json:"true_label"`
		}{{
			Task: "complexity", Labels: labels, TrueLabel: []string{complexity},
		}}
		b, _ := json.Marshal(row)
		if _, err := f.Write(append(b, '\n')); err != nil {
			return "", 0, err
		}
		n++
	}
	if n == 0 {
		return "", 0, fmt.Errorf("no routing events to export")
	}
	return path, n, rows.Err()
}
