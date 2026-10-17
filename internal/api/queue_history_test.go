package api

import (
	"testing"
	"time"
)

func TestQueueHistorySample(t *testing.T) {
	h := newQueueHistory(15 * time.Minute)
	h.Sample(2)
	h.Sample(5)
	s := h.Series()
	if len(s) != 1 {
		t.Fatalf("same-minute collapse want 1 got %d", len(s))
	}
	if s[0].Waiting != 5 {
		t.Fatalf("waiting=%d", s[0].Waiting)
	}
}
