package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// OllamaBalanceQuota fetches https://ollama.com/api/balance (remaining_percent → used).
type OllamaBalanceQuota struct {
	providerID string
	baseURL    string
	apiKey     string
	client     *http.Client
}

func NewOllamaBalanceQuota(providerID, baseURL, apiKey string) *OllamaBalanceQuota {
	if baseURL == "" {
		baseURL = "https://ollama.com"
	}
	return &OllamaBalanceQuota{
		providerID: providerID,
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		client:     &http.Client{Timeout: 15 * time.Second},
	}
}

func (q *OllamaBalanceQuota) ProviderID() string { return q.providerID }

type ollamaBalanceResp struct {
	Included struct {
		Session *struct {
			RemainingPercent float64 `json:"remaining_percent"`
			ResetsAt         string  `json:"resets_at"`
		} `json:"session"`
		Weekly *struct {
			RemainingPercent float64 `json:"remaining_percent"`
			ResetsAt         string  `json:"resets_at"`
		} `json:"weekly"`
	} `json:"included"`
}

func remainingToWindow(remaining float64, resets string) *QuotaWindow {
	w := &QuotaWindow{UsedPercent: 100 - remaining}
	if w.UsedPercent < 0 {
		w.UsedPercent = 0
	}
	if w.UsedPercent > 100 {
		w.UsedPercent = 100
	}
	if resets != "" {
		if t, err := time.Parse(time.RFC3339, resets); err == nil {
			w.ResetsAt = t
		}
	}
	return w
}

func (q *OllamaBalanceQuota) FetchQuota(ctx context.Context) (QuotaSnapshot, error) {
	url := q.baseURL + "/api/balance"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
	if resp.StatusCode != http.StatusOK {
		return QuotaSnapshot{}, fmt.Errorf("ollama balance: HTTP %d", resp.StatusCode)
	}
	var body ollamaBalanceResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return QuotaSnapshot{}, err
	}
	snap := QuotaSnapshot{
		ProviderID: q.providerID,
		FetchedAt:  time.Now().UTC(),
		Source:     "ollama_balance",
	}
	if body.Included.Session != nil {
		snap.Rolling = remainingToWindow(body.Included.Session.RemainingPercent, body.Included.Session.ResetsAt)
	}
	if body.Included.Weekly != nil {
		snap.Weekly = remainingToWindow(body.Included.Weekly.RemainingPercent, body.Included.Weekly.ResetsAt)
	}
	return snap, nil
}
