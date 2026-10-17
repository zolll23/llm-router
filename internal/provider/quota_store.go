package provider

import (
	"context"
	"log"
	"sync"
	"time"
)

// QuotaStore caches quota snapshots and cooldowns after 429.
type QuotaStore struct {
	mu       sync.RWMutex
	fetchers map[string]QuotaFetcher // providerID → fetcher (has adapter)
	snaps    map[string]QuotaSnapshot
	cooldown map[string]time.Time
}

func NewQuotaStore() *QuotaStore {
	return &QuotaStore{
		fetchers: map[string]QuotaFetcher{},
		snaps:    map[string]QuotaSnapshot{},
		cooldown: map[string]time.Time{},
	}
}

func (s *QuotaStore) Register(f QuotaFetcher) {
	if f == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fetchers[f.ProviderID()] = f
}

func (s *QuotaStore) HasAdapter(providerID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.fetchers[providerID]
	return ok
}

func (s *QuotaStore) Refresh(ctx context.Context) {
	s.mu.RLock()
	list := make([]QuotaFetcher, 0, len(s.fetchers))
	for _, f := range s.fetchers {
		list = append(list, f)
	}
	s.mu.RUnlock()
	for _, f := range list {
		snap, err := f.FetchQuota(ctx)
		s.mu.Lock()
		if err != nil {
			log.Printf("quota refresh %s: %v", f.ProviderID(), err)
			prev := s.snaps[f.ProviderID()]
			prev.ProviderID = f.ProviderID()
			prev.Error = err.Error()
			prev.FetchedAt = time.Now().UTC()
			s.snaps[f.ProviderID()] = prev
		} else {
			snap.Error = ""
			s.snaps[f.ProviderID()] = snap
		}
		s.mu.Unlock()
	}
}

func (s *QuotaStore) StartPeriodicRefresh(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = time.Minute
	}
	go func() {
		s.Refresh(ctx)
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.Refresh(ctx)
			}
		}
	}()
}

func (s *QuotaStore) Snapshot(providerID string) (QuotaSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap, ok := s.snaps[providerID]
	return snap, ok
}

func (s *QuotaStore) All() []QuotaSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]QuotaSnapshot, 0, len(s.snaps)+len(s.fetchers))
	seen := map[string]bool{}
	for id, f := range s.fetchers {
		if snap, ok := s.snaps[id]; ok {
			out = append(out, snap)
			seen[id] = true
		} else {
			out = append(out, QuotaSnapshot{ProviderID: f.ProviderID(), Source: "pending"})
			seen[id] = true
		}
	}
	for id, snap := range s.snaps {
		if !seen[id] {
			out = append(out, snap)
		}
	}
	return out
}

func (s *QuotaStore) MarkCooldown(providerID string, until time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cooldown[providerID] = until
}

func (s *QuotaStore) InCooldown(providerID string, now time.Time) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	until, ok := s.cooldown[providerID]
	if !ok {
		return false
	}
	return now.Before(until)
}

func windowOK(w *QuotaWindow, thr float64) bool {
	if w == nil {
		return true // no data for this window
	}
	return w.UsedPercent < thr
}

// HeadroomOK implements spec rules:
// - no adapter → true (unless cooldown)
// - adapter but no successful snap (Error set or never fetched) → false
// - adapter with snap → all windows under thresholds
func (s *QuotaStore) HeadroomOK(providerID string, thr QuotaThresholds, now time.Time) bool {
	if now.IsZero() {
		now = time.Now()
	}
	if s.InCooldown(providerID, now) {
		return false
	}
	s.mu.RLock()
	_, hasAdapter := s.fetchers[providerID]
	snap, hasSnap := s.snaps[providerID]
	s.mu.RUnlock()

	if !hasAdapter {
		return true
	}
	if !hasSnap || snap.Error != "" {
		return false
	}
	if snap.Rolling == nil && snap.Weekly == nil && snap.Monthly == nil {
		return false
	}
	if thr.Rolling <= 0 {
		thr = DefaultQuotaThresholds()
	}
	return windowOK(snap.Rolling, thr.Rolling) &&
		windowOK(snap.Weekly, thr.Weekly) &&
		windowOK(snap.Monthly, thr.Monthly)
}

// ExternalOK is HeadroomOK for escalation scoring.
func (s *QuotaStore) ExternalOK(providerID string, thr QuotaThresholds, now time.Time) bool {
	return s.HeadroomOK(providerID, thr, now)
}
