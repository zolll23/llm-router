package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestAcquireFitsImmediately(t *testing.T) {
	s := NewLocal(20, func(context.Context) (map[string]float64, error) {
		return map[string]float64{}, nil
	})
	_, rel, err := s.Acquire(context.Background(), "gemma2:9b", 6, 8)
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil {
		t.Fatal("nil release")
	}
	rel()
}

func TestAcquireSameLoadedNoExtraBudget(t *testing.T) {
	s := NewLocal(10, func(context.Context) (map[string]float64, error) {
		return map[string]float64{"big:70b": 40}, nil
	})
	// 40 already loaded but same model → admit (in-flight 0)
	_, rel, err := s.Acquire(context.Background(), "big:70b", 40, 8)
	if err != nil {
		t.Fatal(err)
	}
	rel()
}

func TestOverflowWhenQueueFull(t *testing.T) {
	block := make(chan struct{})
	s := NewLocal(8, func(context.Context) (map[string]float64, error) {
		return map[string]float64{"a:7b": 6}, nil
	})
	_, rel, err := s.Acquire(context.Background(), "a:7b", 6, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, err := s.Acquire(ctx, "b:14b", 10, 1)
		if err == nil {
			close(block)
		}
	}()
	// Let first waiter enqueue
	time.Sleep(50 * time.Millisecond)
	_, _, err = s.Acquire(context.Background(), "c:7b", 5, 1)
	if !errors.Is(err, ErrOverflow) {
		t.Fatalf("want overflow, got %v", err)
	}
	cancel()
	wg.Wait()
}

func TestWaitThenAcquire(t *testing.T) {
	s := NewLocal(10, func(context.Context) (map[string]float64, error) {
		return map[string]float64{}, nil
	})
	_, rel1, err := s.Acquire(context.Background(), "m1:7b", 8, 4)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, rel2, err := s.Acquire(context.Background(), "m2:7b", 8, 4)
		if err != nil {
			done <- err
			return
		}
		rel2()
		done <- nil
	}()

	time.Sleep(30 * time.Millisecond)
	if s.WaitingLen() != 1 {
		t.Fatalf("waiting=%d", s.WaitingLen())
	}
	rel1()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for second acquire")
	}
}

func TestEstimateNeedGB(t *testing.T) {
	if g := EstimateNeedGB("x", 14); g < 9 || g > 10 {
		t.Fatalf("params: %v", g)
	}
	if g := EstimateNeedGB("qwen2.5:32b", 0); g < 20 {
		t.Fatalf("name: %v", g)
	}
}

func TestHostBudgetGB(t *testing.T) {
	if g := HostBudgetGB(36, 36, true); g != 25 {
		t.Fatalf("unified: %v", g)
	}
	if g := HostBudgetGB(0, 24, false); g != 24 {
		t.Fatalf("vram: %v", g)
	}
}
