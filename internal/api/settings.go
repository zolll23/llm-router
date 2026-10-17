package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type RuntimeSettings struct {
	Classifier       string  `json:"classifier"` // gliner|nimble|heuristic
	GLiNERBaseURL    string  `json:"gliner_base_url"`
	NimbleModel      string  `json:"nimble_model"`
	NimbleBaseURL    string  `json:"nimble_base_url"`
	StickyTTLMinutes int     `json:"sticky_ttl_minutes"`
	ConfidenceMin    float64 `json:"confidence_min"`
	KeepAlive        string  `json:"keep_alive"`

	EscalateMinComplexity     string  `json:"escalate_min_complexity"`
	QuotaThresholdRollingPct  float64 `json:"quota_threshold_rolling_pct"`
	QuotaThresholdWeeklyPct   float64 `json:"quota_threshold_weekly_pct"`
	QuotaThresholdMonthlyPct  float64 `json:"quota_threshold_monthly_pct"`
	QuotaCooldownSeconds      int     `json:"quota_cooldown_seconds"`

	LocalQueueMax      int  `json:"local_queue_max"`
	LocalQueueRedirect bool `json:"local_queue_redirect"`
}

type SettingsStore struct {
	mu   sync.RWMutex
	path string
	cur  RuntimeSettings
}

func defaultSettings() RuntimeSettings {
	return RuntimeSettings{
		Classifier:                "gliner",
		GLiNERBaseURL:             "http://127.0.0.1:8090",
		NimbleModel:               "nimble",
		NimbleBaseURL:             "http://127.0.0.1:11434",
		StickyTTLMinutes:          30,
		ConfidenceMin:             0.4,
		KeepAlive:                 "60m",
		EscalateMinComplexity:     "hard",
		QuotaThresholdRollingPct:  85,
		QuotaThresholdWeeklyPct:   90,
		QuotaThresholdMonthlyPct:  95,
		QuotaCooldownSeconds:      600,
		LocalQueueMax:             8,
		LocalQueueRedirect:        false,
	}
}

func NewSettingsStore(dataDir string) (*SettingsStore, error) {
	s := &SettingsStore{
		path: filepath.Join(dataDir, "settings.json"),
		cur:  defaultSettings(),
	}
	b, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(b, &s.cur)
	}
	s.applyEnvOverrides()
	if s.cur.Classifier == "" {
		s.cur.Classifier = "gliner"
	}
	if s.cur.GLiNERBaseURL == "" {
		s.cur.GLiNERBaseURL = "http://127.0.0.1:8090"
	}
	s.fillEscalationDefaults()
	return s, nil
}

func (s *SettingsStore) fillEscalationDefaults() {
	d := defaultSettings()
	if s.cur.EscalateMinComplexity == "" {
		s.cur.EscalateMinComplexity = d.EscalateMinComplexity
	}
	if s.cur.QuotaThresholdRollingPct <= 0 {
		s.cur.QuotaThresholdRollingPct = d.QuotaThresholdRollingPct
	}
	if s.cur.QuotaThresholdWeeklyPct <= 0 {
		s.cur.QuotaThresholdWeeklyPct = d.QuotaThresholdWeeklyPct
	}
	if s.cur.QuotaThresholdMonthlyPct <= 0 {
		s.cur.QuotaThresholdMonthlyPct = d.QuotaThresholdMonthlyPct
	}
	if s.cur.QuotaCooldownSeconds <= 0 {
		s.cur.QuotaCooldownSeconds = d.QuotaCooldownSeconds
	}
	if s.cur.LocalQueueMax < 0 {
		s.cur.LocalQueueMax = d.LocalQueueMax
	}
}

func (s *SettingsStore) applyEnvOverrides() {
	if v := os.Getenv("ROUTER_CLASSIFIER"); v != "" {
		s.cur.Classifier = v
	}
	if v := os.Getenv("ROUTER_GLINER_BASE_URL"); v != "" {
		s.cur.GLiNERBaseURL = v
	}
	if v := os.Getenv("ROUTER_NIMBLE_BASE_URL"); v != "" {
		s.cur.NimbleBaseURL = v
	}
	if v := os.Getenv("ROUTER_NIMBLE_MODEL"); v != "" {
		s.cur.NimbleModel = v
	}
}

func (s *SettingsStore) Get() RuntimeSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

func (s *SettingsStore) Put(v RuntimeSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.Classifier == "" {
		v.Classifier = s.cur.Classifier
	}
	if v.GLiNERBaseURL == "" {
		v.GLiNERBaseURL = s.cur.GLiNERBaseURL
	}
	tmp := &SettingsStore{cur: v}
	tmp.fillEscalationDefaults()
	v = tmp.cur
	s.cur = v
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}
