package api

import (
	"sync"
	"time"
)

type queuePoint struct {
	TS      int64 `json:"ts"`
	Waiting int   `json:"waiting"`
}

// queueHistory keeps a short in-memory ring of queue depth samples for the UI.
type queueHistory struct {
	mu      sync.Mutex
	points  []queuePoint
	keepFor time.Duration
}

func newQueueHistory(keepFor time.Duration) *queueHistory {
	if keepFor <= 0 {
		keepFor = 15 * time.Minute
	}
	return &queueHistory{keepFor: keepFor}
}

func (h *queueHistory) Sample(waiting int) {
	if h == nil {
		return
	}
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	// Collapse samples within the same minute.
	bucket := now.Truncate(time.Minute).UnixMilli()
	if n := len(h.points); n > 0 && h.points[n-1].TS == bucket {
		h.points[n-1].Waiting = waiting
	} else {
		h.points = append(h.points, queuePoint{TS: bucket, Waiting: waiting})
	}
	cut := now.Add(-h.keepFor).UnixMilli()
	i := 0
	for i < len(h.points) && h.points[i].TS < cut {
		i++
	}
	if i > 0 {
		h.points = append([]queuePoint{}, h.points[i:]...)
	}
}

func (h *queueHistory) Series() []queuePoint {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]queuePoint, len(h.points))
	copy(out, h.points)
	return out
}
