package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type DB struct {
	SQL *sql.DB
}

func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "router.db")
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	d := &DB{SQL: sqlDB}
	if err := d.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) migrate() error {
	_, err := d.SQL.Exec(`
CREATE TABLE IF NOT EXISTS usage_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  model_id TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  prompt_tokens INTEGER NOT NULL DEFAULT 0,
  cached_tokens INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0,
  latency_ms INTEGER NOT NULL DEFAULT 0,
  session_id TEXT,
  complexity TEXT,
  confidence REAL,
  classifier_source TEXT,
  http_status INTEGER,
  estimated INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_usage_ts_model ON usage_events(ts, model_id);

CREATE TABLE IF NOT EXISTS routing_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  request_id TEXT,
  session_id TEXT,
  state_text TEXT,
  pred_complexity TEXT,
  pred_needs_tools INTEGER,
  pred_confidence REAL,
  classifier_source TEXT,
  chosen_model TEXT,
  cost_tier TEXT,
  reason TEXT,
  escalated INTEGER NOT NULL DEFAULT 0,
  sticky_break INTEGER NOT NULL DEFAULT 0,
  user_override INTEGER NOT NULL DEFAULT 0,
  local_success INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_routing_ts ON routing_events(ts);

CREATE TABLE IF NOT EXISTS judge_labels (
  request_id TEXT PRIMARY KEY,
  gold_complexity TEXT,
  underkill INTEGER,
  overkill INTEGER,
  judge_confidence REAL,
  judge_model TEXT,
  judged_at INTEGER
);
`)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	for _, col := range []struct {
		table, name, decl string
	}{
		{"usage_events", "category", "TEXT"},
		{"usage_events", "pool", "TEXT"},
		{"routing_events", "pred_category", "TEXT"},
		{"routing_events", "pool", "TEXT"},
		{"routing_events", "http_status", "INTEGER"},
	} {
		if err := d.addColumnIfMissing(col.table, col.name, col.decl); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) addColumnIfMissing(table, name, decl string) error {
	rows, err := d.SQL.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var colName, colType string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &colName, &colType, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if strings.EqualFold(colName, name) {
			return nil
		}
	}
	_, err = d.SQL.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, name, decl))
	return err
}

func (d *DB) Close() error { return d.SQL.Close() }

// PoolLabel builds a human pool name like "ollama · local".
func PoolLabel(providerID, costTier string) string {
	if providerID == "" {
		providerID = "unknown"
	}
	if costTier == "" {
		costTier = "unknown"
	}
	return providerID + " · " + costTier
}

// StatusLabel maps HTTP status to a Sankey terminal node.
func StatusLabel(httpStatus int) string {
	switch {
	case httpStatus == 429:
		return "лимит"
	case httpStatus >= 200 && httpStatus < 400:
		return "успех"
	default:
		return "ошибка"
	}
}
