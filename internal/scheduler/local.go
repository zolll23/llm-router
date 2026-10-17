package scheduler

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"router-app/internal/provider"
)

// ErrOverflow means the wait queue is full; caller should redirect or 429.
var ErrOverflow = errors.New("local queue overflow")

// Local is a memory-aware gate for one Ollama provider.
type Local struct {
	mu       sync.Mutex
	budgetGB float64
	fetchPS  func(context.Context) (map[string]float64, error)

	inFlight map[string]int     // normalized name → count (0 or 1)
	reserved map[string]float64 // normalized name → GB while in-flight

	waiters []*waiter
}

type waiter struct {
	key    string
	needGB float64
	wake   chan struct{}
}

// NewLocal builds a scheduler. budgetGB <= 0 defaults to 12.
func NewLocal(budgetGB float64, fetchPS func(context.Context) (map[string]float64, error)) *Local {
	if budgetGB <= 0 {
		budgetGB = 12
	}
	if fetchPS == nil {
		fetchPS = func(context.Context) (map[string]float64, error) {
			return map[string]float64{}, nil
		}
	}
	return &Local{
		budgetGB: budgetGB,
		fetchPS:  fetchPS,
		inFlight: map[string]int{},
		reserved: map[string]float64{},
	}
}

// HostBudgetGB mirrors recommendations: unified → RAM*0.72, else VRAM.
func HostBudgetGB(ramGB, vramGB int, unified bool) float64 {
	if unified {
		if ramGB <= 0 && vramGB > 0 {
			ramGB = vramGB
		}
		if ramGB <= 0 {
			return 12
		}
		return math.Floor(float64(ramGB) * 0.72)
	}
	if vramGB > 0 {
		return float64(vramGB)
	}
	if ramGB > 0 {
		return float64(ramGB)
	}
	return 12
}

var paramsRe = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*b\b`)

// EstimateNeedGB estimates working-set GB from params or model name.
func EstimateNeedGB(upstreamID string, params int) float64 {
	if params > 0 {
		req := float64(params) * 0.7
		if req < 2 {
			req = 2
		}
		return req
	}
	m := paramsRe.FindStringSubmatch(upstreamID)
	if len(m) == 2 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > 0 {
			req := v * 0.7
			if req < 2 {
				req = 2
			}
			return req
		}
	}
	return 8
}

// WaitingLen returns current wait-queue depth (for tests/metrics).
func (s *Local) WaitingLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.waiters)
}

// Acquire blocks until the model can run, or returns ErrOverflow / ctx error.
func (s *Local) Acquire(ctx context.Context, upstreamID string, needGB float64, queueMax int) (waitMS int64, release func(), err error) {
	if needGB <= 0 {
		needGB = 8
	}
	key := norm(upstreamID)
	start := time.Now()

	var w *waiter
	enqueued := false

	for {
		if err := ctx.Err(); err != nil {
			s.mu.Lock()
			if enqueued {
				s.removeWaiter(w)
			}
			s.mu.Unlock()
			return 0, nil, err
		}

		ps := s.safePS(ctx)

		s.mu.Lock()
		if s.canAdmitLocked(key, needGB, ps) {
			if enqueued {
				s.removeWaiter(w)
			}
			s.reserveLocked(key, needGB)
			s.mu.Unlock()
			rel := s.makeRelease(key)
			return time.Since(start).Milliseconds(), rel, nil
		}

		if !enqueued {
			if queueMax <= 0 || len(s.waiters) >= queueMax {
				s.mu.Unlock()
				return 0, nil, ErrOverflow
			}
			w = &waiter{key: key, needGB: needGB, wake: make(chan struct{}, 1)}
			s.waiters = append(s.waiters, w)
			enqueued = true
		}
		s.mu.Unlock()

		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.removeWaiter(w)
			s.mu.Unlock()
			return 0, nil, ctx.Err()
		case <-w.wake:
			// retry admit
		}
	}
}

func (s *Local) makeRelease(key string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			ps := s.safePS(context.Background())
			s.mu.Lock()
			s.inFlight[key]--
			if s.inFlight[key] <= 0 {
				delete(s.inFlight, key)
				delete(s.reserved, key)
			}
			s.wakeLocked(ps)
			s.mu.Unlock()
		})
	}
}

func (s *Local) safePS(ctx context.Context) map[string]float64 {
	ps, err := s.fetchPS(ctx)
	if err != nil || ps == nil {
		return map[string]float64{}
	}
	out := make(map[string]float64, len(ps))
	for k, v := range ps {
		out[norm(k)] = v
	}
	return out
}

func (s *Local) canAdmitLocked(key string, needGB float64, ps map[string]float64) bool {
	if s.inFlight[key] >= 1 {
		return false
	}
	if _, loaded := ps[key]; loaded {
		return true
	}
	// also match via provider helper on raw keys — ps already normalized
	for name := range ps {
		if provider.ModelNameMatch(name, key) {
			return true
		}
	}
	used := s.usedGBLocked(ps)
	return used+needGB <= s.budgetGB+1e-6
}

func (s *Local) usedGBLocked(ps map[string]float64) float64 {
	used := 0.0
	seen := map[string]bool{}
	for name, sz := range ps {
		used += sz
		seen[name] = true
	}
	for name, sz := range s.reserved {
		if !seen[name] {
			used += sz
		}
	}
	return used
}

func (s *Local) reserveLocked(key string, needGB float64) {
	s.inFlight[key] = 1
	s.reserved[key] = needGB
}

func (s *Local) removeWaiter(w *waiter) {
	if w == nil {
		return
	}
	out := s.waiters[:0]
	for _, x := range s.waiters {
		if x != w {
			out = append(out, x)
		}
	}
	s.waiters = out
}

func (s *Local) wakeLocked(ps map[string]float64) {
	if len(s.waiters) == 0 {
		return
	}
	// Prefer waiters whose model is already loaded and free.
	for _, w := range s.waiters {
		if s.inFlight[w.key] > 0 {
			continue
		}
		if _, ok := ps[w.key]; ok {
			signal(w)
			return
		}
	}
	for _, w := range s.waiters {
		if s.inFlight[w.key] == 0 {
			signal(w)
			return
		}
	}
	signal(s.waiters[0])
}

func signal(w *waiter) {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func norm(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexByte(s, '@'); i >= 0 {
		s = s[:i]
	}
	return s
}
