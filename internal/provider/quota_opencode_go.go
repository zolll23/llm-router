package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// OpenCodeGoQuota fetches GET …/zen/go/v1/usage (or {base}/usage if base already ends with /zen/go/v1).
type OpenCodeGoQuota struct {
	providerID string
	baseURL    string
	apiKey     string
	client     *http.Client
}

func NewOpenCodeGoQuota(providerID, baseURL, apiKey string) *OpenCodeGoQuota {
	return &OpenCodeGoQuota{
		providerID: providerID,
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		client:     &http.Client{Timeout: 15 * time.Second},
	}
}

func (q *OpenCodeGoQuota) ProviderID() string { return q.providerID }

func (q *OpenCodeGoQuota) usageURL() string {
	b := q.baseURL
	if strings.HasSuffix(b, "/zen/go/v1") || strings.HasSuffix(b, "/go/v1") {
		return b + "/usage"
	}
	if strings.Contains(b, "/zen/go") {
		return b + "/usage"
	}
	return b + "/zen/go/v1/usage"
}

type goUsageResp struct {
	Usage struct {
		Rolling *goWindow `json:"rolling"`
		Weekly  *goWindow `json:"weekly"`
		Monthly *goWindow `json:"monthly"`
	} `json:"usage"`
}

type goWindow struct {
	Percent  float64 `json:"percent"`
	ResetsAt string  `json:"resetsAt"`
	Status   string  `json:"status"`
}

func mapGoWindow(w *goWindow) *QuotaWindow {
	if w == nil {
		return nil
	}
	out := &QuotaWindow{UsedPercent: w.Percent, Status: w.Status}
	if w.ResetsAt != "" {
		if t, err := time.Parse(time.RFC3339, w.ResetsAt); err == nil {
			out.ResetsAt = t
		}
	}
	return out
}

func (q *OpenCodeGoQuota) FetchQuota(ctx context.Context) (QuotaSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.usageURL(), nil)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	if q.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+q.apiKey)
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return QuotaSnapshot{}, fmt.Errorf("opencode go usage: unauthorized")
	}
	if resp.StatusCode == http.StatusForbidden {
		return QuotaSnapshot{}, fmt.Errorf("opencode go usage: forbidden (no Go subscription?)")
	}
	if resp.StatusCode != http.StatusOK {
		return QuotaSnapshot{}, fmt.Errorf("opencode go usage: HTTP %d", resp.StatusCode)
	}
	var body goUsageResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return QuotaSnapshot{}, err
	}
	return QuotaSnapshot{
		ProviderID: q.providerID,
		Rolling:    mapGoWindow(body.Usage.Rolling),
		Weekly:     mapGoWindow(body.Usage.Weekly),
		Monthly:    mapGoWindow(body.Usage.Monthly),
		FetchedAt:  time.Now().UTC(),
		Source:     "opencode_go",
	}, nil
}
