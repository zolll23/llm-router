package router

import (
	"context"
	"testing"
	"time"

	"router-app/internal/classifier"
	"router-app/internal/provider"
)

func TestDecideSkipsNonAgentOKWhenTools(t *testing.T) {
	models := []provider.ModelInfo{
		{ProviderID: "ollama", UpstreamID: "tiny:7b", Routable: true, Tools: true, CostTier: "local", Params: 7},
		{ProviderID: "ollama", UpstreamID: "big:31b", Routable: true, Tools: true, CostTier: "local", Params: 31},
	}
	agentOK := func(m provider.ModelInfo) bool {
		return provider.EvalAgentOK(m, 14, "").OK
	}
	out, err := Decide(context.Background(), DecideInput{
		Class:      classifier.Result{Category: "coding", Complexity: "medium", NeedsTools: true},
		Models:     models,
		AgentOK:    agentOK,
		EscalateMin: "hard",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Model.UpstreamID != "big:31b" {
		t.Fatalf("got %s", out.Model.UpstreamID)
	}
}

func TestStickyBreakAgentOK(t *testing.T) {
	models := []provider.ModelInfo{
		{ProviderID: "ollama", UpstreamID: "tiny:7b", Routable: true, Tools: true, CostTier: "local", Params: 7},
		{ProviderID: "ollama", UpstreamID: "big:31b", Routable: true, Tools: true, CostTier: "local", Params: 31},
	}
	st := NewStickyStore(time.Hour)
	now := time.Now()
	st.Set("s1", "ollama/tiny:7b", now)
	out, err := Decide(context.Background(), DecideInput{
		Class:      classifier.Result{Category: "coding", Complexity: "medium", NeedsTools: true},
		Models:     models,
		SessionKey: "s1",
		Sticky:     st,
		Now:        now,
		AgentOK: func(m provider.ModelInfo) bool {
			return provider.EvalAgentOK(m, 14, "").OK
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.StickyBreak || out.Model.UpstreamID != "big:31b" {
		t.Fatalf("%+v", out)
	}
}
