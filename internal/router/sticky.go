package router

import (
	"sync"
	"time"
)

type StickyStore struct {
	mu   sync.Mutex
	ttl  time.Duration
	pins map[string]stickyPin
}

type stickyPin struct {
	ModelID string
	Until   time.Time
}

func NewStickyStore(ttl time.Duration) *StickyStore {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &StickyStore{ttl: ttl, pins: map[string]stickyPin{}}
}

func (s *StickyStore) Get(session string, now time.Time) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pins[session]
	if !ok || now.After(p.Until) {
		delete(s.pins, session)
		return "", false
	}
	return p.ModelID, true
}

func (s *StickyStore) Set(session, modelID string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pins[session] = stickyPin{ModelID: modelID, Until: now.Add(s.ttl)}
}

func (s *StickyStore) Clear(session string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pins, session)
}
