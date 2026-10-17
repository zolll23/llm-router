package provider_test

import (
	"testing"

	"router/internal/provider"
)

func TestCanonicalID(t *testing.T) {
	got := provider.CanonicalID("ollama", "qwen3-coder")
	if got != "ollama/qwen3-coder" {
		t.Fatalf("got %q", got)
	}
	m := provider.ModelInfo{ProviderID: "openai", UpstreamID: "gpt-5.2"}
	if m.ID() != "openai/gpt-5.2" {
		t.Fatalf("ModelInfo.ID: %q", m.ID())
	}
}
