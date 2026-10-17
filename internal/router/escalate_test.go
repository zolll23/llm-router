package router_test

import (
	"context"
	"testing"

	"router-app/internal/classifier"
	"router-app/internal/provider"
	"router-app/internal/router"
)

func TestEscalateHardPicksMeteredWhenOK(t *testing.T) {
	models := []provider.ModelInfo{
		{ProviderID: "ollama", UpstreamID: "local", Routable: true, Tools: true, CostTier: "local", Params: 14},
		{ProviderID: "go", UpstreamID: "cloud", Routable: true, Tools: true, CostTier: "metered", Params: 70},
	}
	out, err := router.Decide(context.Background(), router.DecideInput{
		Class:       classifier.Result{Complexity: "hard", NeedsTools: true},
		Models:      models,
		EscalateMin: "hard",
		ExternalOK:  func(pid string) bool { return pid == "go" },
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Model.ID() != "go/cloud" {
		t.Fatalf("got %s want go/cloud", out.Model.ID())
	}
}

func TestEscalateHardFallsBackLocalWhenNoHeadroom(t *testing.T) {
	models := []provider.ModelInfo{
		{ProviderID: "ollama", UpstreamID: "local", Routable: true, Tools: true, CostTier: "local", Params: 14},
		{ProviderID: "go", UpstreamID: "cloud", Routable: true, Tools: true, CostTier: "metered", Params: 70},
	}
	out, err := router.Decide(context.Background(), router.DecideInput{
		Class:       classifier.Result{Complexity: "hard"},
		Models:      models,
		EscalateMin: "hard",
		ExternalOK:  func(string) bool { return false },
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Model.CostTier != "local" {
		t.Fatalf("got %s", out.Model.ID())
	}
}

func TestSimpleStaysLocalEvenIfExternalOK(t *testing.T) {
	models := []provider.ModelInfo{
		{ProviderID: "ollama", UpstreamID: "local", Routable: true, Tools: true, CostTier: "local", Params: 7},
		{ProviderID: "go", UpstreamID: "cloud", Routable: true, Tools: true, CostTier: "metered", Params: 70},
	}
	out, err := router.Decide(context.Background(), router.DecideInput{
		Class:       classifier.Result{Complexity: "simple"},
		Models:      models,
		EscalateMin: "hard",
		ExternalOK:  func(string) bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Model.CostTier != "local" {
		t.Fatalf("got %s", out.Model.ID())
	}
}

func TestLocalOnlyRetry(t *testing.T) {
	models := []provider.ModelInfo{
		{ProviderID: "ollama", UpstreamID: "local", Routable: true, Tools: true, CostTier: "local", Params: 14},
		{ProviderID: "go", UpstreamID: "cloud", Routable: true, Tools: true, CostTier: "metered", Params: 70},
	}
	out, err := router.Decide(context.Background(), router.DecideInput{
		Class:       classifier.Result{Complexity: "hard"},
		Models:      models,
		EscalateMin: "hard",
		LocalOnly:   true,
		ExternalOK:  func(string) bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Model.CostTier != "local" {
		t.Fatalf("got %s", out.Model.ID())
	}
}
