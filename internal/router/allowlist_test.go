package router

import (
	"context"
	"testing"
	"time"

	"router-app/internal/classifier"
	"router-app/internal/provider"
)

func TestAllowlistFiltersLocal(t *testing.T) {
	models := []provider.ModelInfo{
		{ProviderID: "ollama", UpstreamID: "gpt-oss:20b", Routable: true, Tools: true, CostTier: "local", Params: 20},
		{ProviderID: "ollama", UpstreamID: "tiny:7b", Routable: true, Tools: true, CostTier: "local", Params: 7},
		{ProviderID: "cloud", UpstreamID: "big", Routable: true, Tools: true, CostTier: "metered", Params: 0},
	}
	allow := AllowlistForCategory(map[string][]string{
		"coding": {"ollama/gpt-oss:20b"},
	}, "coding")
	loaded := map[string]bool{"ollama/gpt-oss:20b": true}
	out, err := Decide(context.Background(), DecideInput{
		Class:      classifier.Result{Category: "coding", Complexity: "medium", NeedsTools: true},
		Models:     models,
		LocalAllow: allow,
		LocalLoaded: func(m provider.ModelInfo) bool {
			return loaded[m.ID()]
		},
		AgentOK: func(m provider.ModelInfo) bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Model.ID() != "ollama/gpt-oss:20b" {
		t.Fatalf("got %s", out.Model.ID())
	}
}

func TestAllowlistUnloadedGoesCloud(t *testing.T) {
	models := []provider.ModelInfo{
		{ProviderID: "ollama", UpstreamID: "gpt-oss:20b", Routable: true, Tools: true, CostTier: "local", Params: 20},
		{ProviderID: "cloud", UpstreamID: "big", Routable: true, Tools: true, CostTier: "metered", Params: 70},
	}
	allow := map[string]bool{"ollama/gpt-oss:20b": true}
	out, err := Decide(context.Background(), DecideInput{
		Class:      classifier.Result{Category: "coding", Complexity: "medium", NeedsTools: true},
		Models:     models,
		LocalAllow: allow,
		LocalLoaded: func(provider.ModelInfo) bool {
			return false
		},
		ExternalOK: func(string) bool { return true },
		EscalateMin: "trivial",
		AgentOK:     func(m provider.ModelInfo) bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Model.CostTier != "metered" {
		t.Fatalf("want cloud, got %s %s", out.Model.CostTier, out.Model.ID())
	}
}

func TestStickyBreakAllowlist(t *testing.T) {
	st := NewStickyStore(time.Hour)
	st.Set("s1", "ollama/tiny:7b", time.Now())
	models := []provider.ModelInfo{
		{ProviderID: "ollama", UpstreamID: "tiny:7b", Routable: true, Tools: true, CostTier: "local", Params: 7},
		{ProviderID: "ollama", UpstreamID: "gpt-oss:20b", Routable: true, Tools: true, CostTier: "local", Params: 20},
	}
	out, err := Decide(context.Background(), DecideInput{
		Class:      classifier.Result{Category: "coding", Complexity: "medium", NeedsTools: true},
		Models:     models,
		SessionKey: "s1",
		Sticky:     st,
		LocalAllow: map[string]bool{"ollama/gpt-oss:20b": true},
		LocalLoaded: func(m provider.ModelInfo) bool {
			return m.ID() == "ollama/gpt-oss:20b"
		},
		AgentOK: func(m provider.ModelInfo) bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.StickyBreak || out.Model.ID() != "ollama/gpt-oss:20b" {
		t.Fatalf("%+v", out)
	}
}

func TestEmptyAllowlistNoFilter(t *testing.T) {
	if AllowlistForCategory(nil, "coding") != nil {
		t.Fatal("expected nil")
	}
	if AllowlistForCategory(map[string][]string{"coding": {}}, "coding") != nil {
		t.Fatal("expected nil for empty")
	}
}
