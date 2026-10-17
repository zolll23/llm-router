package provider

import (
	"os"
	"strings"

	"router-app/internal/config"
)

// NewQuotaFetcherFromConfig builds an optional quota adapter for a provider config.
func NewQuotaFetcherFromConfig(pc config.ProviderConfig) QuotaFetcher {
	kind := strings.ToLower(strings.TrimSpace(pc.Quota))
	if kind == "" || kind == "none" {
		return nil
	}
	key := ""
	if pc.APIKeyEnv != "" {
		key = os.Getenv(pc.APIKeyEnv)
	}
	switch kind {
	case "opencode_go":
		base := pc.BaseURL
		if base == "" {
			base = "https://opencode.ai"
		}
		return NewOpenCodeGoQuota(pc.ID, base, key)
	case "ollama_balance":
		base := "https://ollama.com"
		return NewOllamaBalanceQuota(pc.ID, base, key)
	default:
		return nil
	}
}
