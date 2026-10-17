package provider

import (
	"context"
	"testing"
	"time"
)

type stubQuota struct {
	id   string
	snap QuotaSnapshot
	err  error
}

func (s *stubQuota) ProviderID() string { return s.id }
func (s *stubQuota) FetchQuota(context.Context) (QuotaSnapshot, error) {
	return s.snap, s.err
}

func TestHeadroomOK_AllUnderThreshold(t *testing.T) {
	s := NewQuotaStore()
	s.Register(&stubQuota{id: "go"})
	s.mu.Lock()
	s.snaps["go"] = QuotaSnapshot{
		ProviderID: "go",
		Rolling:    &QuotaWindow{UsedPercent: 10},
		Weekly:     &QuotaWindow{UsedPercent: 14},
		Monthly:    &QuotaWindow{UsedPercent: 10},
	}
	s.mu.Unlock()
	thr := DefaultQuotaThresholds()
	if !s.HeadroomOK("go", thr, time.Now()) {
		t.Fatal("expected headroom")
	}
}

func TestHeadroomOK_FailsWhenWeeklyOver(t *testing.T) {
	s := NewQuotaStore()
	s.Register(&stubQuota{id: "go"})
	s.mu.Lock()
	s.snaps["go"] = QuotaSnapshot{
		ProviderID: "go",
		Weekly:     &QuotaWindow{UsedPercent: 95},
	}
	s.mu.Unlock()
	if s.HeadroomOK("go", DefaultQuotaThresholds(), time.Now()) {
		t.Fatal("expected no headroom")
	}
}

func TestHeadroomOK_CooldownBlocks(t *testing.T) {
	s := NewQuotaStore()
	s.MarkCooldown("x", time.Now().Add(time.Hour))
	if s.HeadroomOK("x", DefaultQuotaThresholds(), time.Now()) {
		t.Fatal("cooldown should block")
	}
}

func TestHeadroomOK_NoSnapshotDisablesEscalation(t *testing.T) {
	s := NewQuotaStore()
	s.Register(&stubQuota{id: "go"})
	if s.HeadroomOK("go", DefaultQuotaThresholds(), time.Now()) {
		t.Fatal("adapter without snap should disable")
	}
}

func TestHeadroomOK_NoAdapterAllows(t *testing.T) {
	s := NewQuotaStore()
	if !s.HeadroomOK("local-only", DefaultQuotaThresholds(), time.Now()) {
		t.Fatal("no adapter should allow")
	}
}
