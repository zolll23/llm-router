package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"router-app/internal/provider"
)

// ModelOverridesStore persists per-model agent_ok overrides.
type ModelOverridesStore struct {
	mu   sync.RWMutex
	path string
	cur  map[string]string // id -> auto|allow|deny
}

func NewModelOverridesStore(dataDir string) (*ModelOverridesStore, error) {
	s := &ModelOverridesStore{
		path: filepath.Join(dataDir, "model_overrides.json"),
		cur:  map[string]string{},
	}
	b, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(b, &s.cur)
	}
	if s.cur == nil {
		s.cur = map[string]string{}
	}
	return s, nil
}

func (s *ModelOverridesStore) Get(id string) string {
	if s == nil {
		return provider.OverrideAuto
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := s.cur[id]
	if v == "" {
		return provider.OverrideAuto
	}
	return v
}

func (s *ModelOverridesStore) All() map[string]string {
	if s == nil {
		return map[string]string{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.cur))
	for k, v := range s.cur {
		out[k] = v
	}
	return out
}

func (s *ModelOverridesStore) Put(id, override string) error {
	switch override {
	case provider.OverrideAuto, provider.OverrideAllow, provider.OverrideDeny:
	default:
		override = provider.OverrideAuto
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if override == provider.OverrideAuto {
		delete(s.cur, id)
	} else {
		s.cur[id] = override
	}
	b, err := json.MarshalIndent(s.cur, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}
