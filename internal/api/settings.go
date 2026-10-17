package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type RuntimeSettings struct {
	Classifier       string  `json:"classifier"` // nimble|heuristic
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

func NewSettingsStore(dataDir string) (*SettingsStore, error) {
	s := &SettingsStore{
		path: filepath.Join(dataDir, "settings.json"),
		cur: RuntimeSettings{
			Classifier:       "nimble",
			NimbleModel:      "nimble",
			NimbleBaseURL:    "http://127.0.0.1:11434",
			StickyTTLMinutes: 30,
			ConfidenceMin:    0.4,
			KeepAlive:        "60m",
		},
	}
	b, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(b, &s.cur)
	}
	return s, nil
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
	s.cur = v
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}
