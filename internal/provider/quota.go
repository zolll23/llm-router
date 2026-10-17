package provider

import (
	"context"
	"time"
)

// QuotaWindow is one usage meter (percent used 0–100).
type QuotaWindow struct {
	UsedPercent float64   `json:"used_percent"`
	ResetsAt    time.Time `json:"resets_at,omitempty"`
	Status      string    `json:"status,omitempty"`
}

// QuotaSnapshot is the last known quota state for a provider.
type QuotaSnapshot struct {
	ProviderID string       `json:"provider_id"`
	Rolling    *QuotaWindow `json:"rolling,omitempty"`
	Weekly     *QuotaWindow `json:"weekly,omitempty"`
	Monthly    *QuotaWindow `json:"monthly,omitempty"`
	FetchedAt  time.Time    `json:"fetched_at"`
	Source     string       `json:"source,omitempty"`
	Error      string       `json:"error,omitempty"`
}

// QuotaFetcher pulls account/provider quotas. Optional — not all providers implement it.
type QuotaFetcher interface {
	ProviderID() string
	FetchQuota(ctx context.Context) (QuotaSnapshot, error)
}

// QuotaThresholds are max allowed used% before escalation is blocked.
type QuotaThresholds struct {
	Rolling float64
	Weekly  float64
	Monthly float64
}

func DefaultQuotaThresholds() QuotaThresholds {
	return QuotaThresholds{Rolling: 85, Weekly: 90, Monthly: 95}
}
