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
}

type SettingsStore struct {
	mu   sync.RWMutex
	path string
	cur  RuntimeSettings
}

func defaultSettings() RuntimeSettings {
	return RuntimeSettings{
		Classifier:       "gliner",
		GLiNERBaseURL:    "http://127.0.0.1:8090",
		NimbleModel:      "nimble",
		NimbleBaseURL:    "http://127.0.0.1:11434",
		StickyTTLMinutes: 30,
		ConfidenceMin:    0.4,
		KeepAlive:        "60m",
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
	return s, nil
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
	s.cur = v
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}
