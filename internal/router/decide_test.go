package router_test

import (
	"context"
	"testing"
	"time"

	"router-app/internal/classifier"
	"router-app/internal/provider"
	"router-app/internal/router"
)

func TestDecidePrefersLocalForSimple(t *testing.T) {
	models := []provider.ModelInfo{
		{ProviderID: "openai", UpstreamID: "gpt", Routable: true, Tools: true, CostTier: "metered", Context: 128000},
		{ProviderID: "ollama", UpstreamID: "llama", Routable: true, Tools: true, CostTier: "local", Context: 128000},
	}
	out, err := router.Decide(context.Background(), router.DecideInput{
		Class:  classifier.Result{Complexity: "simple", NeedsTools: true},
		Models: models,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Model.ID() != "ollama/llama" {
		t.Fatalf("got %s", out.Model.ID())
	}
}

func TestSticky(t *testing.T) {
	models := []provider.ModelInfo{
		{ProviderID: "ollama", UpstreamID: "a", Routable: true, Tools: true, CostTier: "local", Context: 128000},
		{ProviderID: "ollama", UpstreamID: "b", Routable: true, Tools: true, CostTier: "local", Context: 128000},
	}
	st := router.NewStickyStore(time.Hour)
	now := time.Now()
	first, err := router.Decide(context.Background(), router.DecideInput{
		Class: classifier.Result{Complexity: "simple"}, Models: models, SessionKey: "s1", Sticky: st, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := router.Decide(context.Background(), router.DecideInput{
		Class: classifier.Result{Complexity: "hard"}, Models: models, SessionKey: "s1", Sticky: st, Now: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !second.StickyHit || second.Model.ID() != first.Model.ID() {
		t.Fatalf("sticky failed: first=%s second=%+v", first.Model.ID(), second)
	}
}
