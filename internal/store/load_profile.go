package store

import "time"

type LoadCell struct {
	Category   string  `json:"category"`
	Complexity string  `json:"complexity"`
	Requests   int64   `json:"requests"`
	SharePct   float64 `json:"share_pct"`
}

// LoadProfile returns category×complexity request counts for the last `days`.
func (d *DB) LoadProfile(days int) ([]LoadCell, int64, error) {
	if days <= 0 {
		days = 7
	}
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
	rows, err := d.SQL.Query(`
SELECT
  COALESCE(NULLIF(TRIM(category), ''), 'chat') AS category,
  COALESCE(NULLIF(TRIM(complexity), ''), 'simple') AS complexity,
  COUNT(*) AS n
FROM usage_events
WHERE ts >= ?
GROUP BY 1, 2
ORDER BY n DESC`, since)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var cells []LoadCell
	var total int64
	for rows.Next() {
		var c LoadCell
		if err := rows.Scan(&c.Category, &c.Complexity, &c.Requests); err != nil {
			return nil, 0, err
		}
		total += c.Requests
		cells = append(cells, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if total > 0 {
		for i := range cells {
			cells[i].SharePct = float64(cells[i].Requests) * 100 / float64(total)
		}
	}
	return cells, total, nil
}
