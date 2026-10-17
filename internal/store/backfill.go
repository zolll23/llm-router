package store

type RoutingBackfillRow struct {
	ID          int64
	StateText   string
	ChosenModel string
	CostTier    string
}

func (d *DB) ListRoutingNeedingCategory(limit int) ([]RoutingBackfillRow, error) {
	if limit <= 0 {
		limit = 5000
	}
	rows, err := d.SQL.Query(`
SELECT id, COALESCE(state_text,''), COALESCE(chosen_model,''), COALESCE(cost_tier,'')
FROM routing_events
WHERE pred_category IS NULL OR TRIM(pred_category) = ''
ORDER BY id ASC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RoutingBackfillRow
	for rows.Next() {
		var r RoutingBackfillRow
		if err := rows.Scan(&r.ID, &r.StateText, &r.ChosenModel, &r.CostTier); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) UpdateRoutingCategory(id int64, category string) error {
	_, err := d.SQL.Exec(`UPDATE routing_events SET pred_category=? WHERE id=?`, category, id)
	return err
}

func (d *DB) BackfillPools() (int64, error) {
	res, err := d.SQL.Exec(`
UPDATE routing_events
SET pool = CASE
  WHEN instr(chosen_model, '/') > 0
    THEN substr(chosen_model, 1, instr(chosen_model, '/') - 1) || ' · ' || COALESCE(NULLIF(TRIM(cost_tier), ''), 'unknown')
  ELSE 'unknown · ' || COALESCE(NULLIF(TRIM(cost_tier), ''), 'unknown')
END
WHERE pool IS NULL OR TRIM(pool) = ''`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()

	res2, err := d.SQL.Exec(`
UPDATE usage_events
SET pool = CASE
  WHEN instr(model_id, '/') > 0
    THEN substr(model_id, 1, instr(model_id, '/') - 1) || ' · local'
  ELSE COALESCE(NULLIF(TRIM(provider_id), ''), 'unknown') || ' · unknown'
END
WHERE pool IS NULL OR TRIM(pool) = ''`)
	if err != nil {
		return n, err
	}
	n2, _ := res2.RowsAffected()
	return n + n2, nil
}

func (d *DB) SyncUsageCategoryFromRouting() (int64, error) {
	res, err := d.SQL.Exec(`
UPDATE usage_events
SET category = (
  SELECT r.pred_category FROM routing_events r WHERE r.id = usage_events.id
)
WHERE (category IS NULL OR TRIM(category) = '')
  AND EXISTS (
    SELECT 1 FROM routing_events r
    WHERE r.id = usage_events.id
      AND r.pred_category IS NOT NULL AND TRIM(r.pred_category) != ''
  )`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (d *DB) SyncUsagePoolFromRouting() (int64, error) {
	res, err := d.SQL.Exec(`
UPDATE usage_events
SET pool = (
  SELECT r.pool FROM routing_events r WHERE r.id = usage_events.id
)
WHERE (pool IS NULL OR TRIM(pool) = '')
  AND EXISTS (
    SELECT 1 FROM routing_events r
    WHERE r.id = usage_events.id
      AND r.pool IS NOT NULL AND TRIM(r.pool) != ''
  )`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (d *DB) CategoryCounts() (map[string]int, error) {
	rows, err := d.SQL.Query(`
SELECT COALESCE(NULLIF(TRIM(category), ''), '(empty)'), COUNT(*)
FROM usage_events GROUP BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}
