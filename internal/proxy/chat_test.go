package proxy_test

import (
	"testing"

	"router/internal/provider"
	"router/internal/proxy"
)

func TestPickFirstLocal(t *testing.T) {
	models := []provider.ModelInfo{
		{ProviderID: "openai", UpstreamID: "gpt", Routable: true, CostTier: "metered"},
		{ProviderID: "ollama", UpstreamID: "llama", Routable: true, CostTier: "local"},
	}
	m, ok := proxy.PickFirstLocal(models)
	if !ok || m.ID() != "ollama/llama" {
		t.Fatalf("got %+v ok=%v", m, ok)
	}
}
