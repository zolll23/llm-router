package scheduler

import "sync"

// Registry maps provider id → Local scheduler.
type Registry struct {
	mu sync.RWMutex
	m  map[string]*Local
}

func NewRegistry() *Registry {
	return &Registry{m: map[string]*Local{}}
}

func (r *Registry) Register(providerID string, s *Local) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[providerID] = s
}

func (r *Registry) Get(providerID string) *Local {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.m[providerID]
}
