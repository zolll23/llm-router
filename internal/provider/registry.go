package provider

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"router-app/internal/config"
)

type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	models    []ModelInfo
	byID      map[string]ModelInfo
	perf      *PerfStore
}

func NewRegistry() *Registry {
	return &Registry{
		providers: map[string]Provider{},
		byID:      map[string]ModelInfo{},
	}
}

func (r *Registry) SetPerfStore(p *PerfStore) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.perf = p
}

func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.ID()] = p
}

func NewFromConfig(pc config.ProviderConfig) (Provider, error) {
	switch pc.Type {
	case "ollama":
		return NewOllama(pc.ID, pc.BaseURL), nil
	case "openai_compatible":
		return NewOpenAICompat(pc.ID, pc.BaseURL, pc.APIKeyEnv), nil
	case "opencode_config":
		return NewOpenCodeConfig(pc.ID, pc.ConfigPath), nil
	case "cursor_cli":
		cmd := pc.Command
		if cmd == "" {
			cmd = "agent"
		}
		return NewCursorCLI(pc.ID, cmd), nil
	default:
		return nil, fmt.Errorf("unknown provider type %q", pc.Type)
	}
}

func (r *Registry) Sync(ctx context.Context) error {
	r.mu.RLock()
	provs := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		provs = append(provs, p)
	}
	r.mu.RUnlock()

	var all []ModelInfo
	byID := map[string]ModelInfo{}
	for _, p := range provs {
		models, err := p.ListModels(ctx)
		if err != nil {
			log.Printf("inventory sync %s: %v", p.ID(), err)
			continue
		}
		for _, m := range models {
			if m.Params == 0 {
				m.Params = EstimateParamsFromName(m.UpstreamID)
				if m.Params == 0 {
					m.Params = EstimateParamsFromName(m.DisplayName)
				}
			}
			all = append(all, m)
			byID[m.ID()] = m
		}
	}
	r.mu.Lock()
	r.models = all
	r.byID = byID
	r.mu.Unlock()
	return nil
}

func (r *Registry) StartPeriodicSync(ctx context.Context, every time.Duration) {
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = r.Sync(ctx)
			}
		}
	}()
}

func (r *Registry) Models() []ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ModelInfo, len(r.models))
	copy(out, r.models)
	perf := r.perf
	if perf != nil {
		for i := range out {
			perf.Apply(&out[i])
		}
	}
	return out
}

func (r *Registry) Get(canonicalID string) (ModelInfo, Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.byID[canonicalID]
	if !ok {
		return ModelInfo{}, nil, false
	}
	p, ok := r.providers[m.ProviderID]
	if !ok {
		return ModelInfo{}, nil, false
	}
	if r.perf != nil {
		r.perf.Apply(&m)
	}
	return m, p, true
}

func (r *Registry) Provider(id string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}
