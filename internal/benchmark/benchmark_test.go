package benchmark_test

import (
	"testing"

	"router-app/internal/benchmark"
	"router-app/internal/provider"
	"router-app/internal/store"
)

func TestRecommendMacM5PrefersMLXByCategory(t *testing.T) {
	s := benchmark.NewStore("")
	_ = s.Update()
	hw := benchmark.PresetMacM536
	inv := []provider.ModelInfo{
		{ProviderID: "ollama", UpstreamID: "gemma4:31b-mlx", Routable: true, CostTier: "local", Params: 31},
		{ProviderID: "ollama", UpstreamID: "qwen2.5-coder:14b-mlx", Routable: true, CostTier: "local", Params: 14},
		{ProviderID: "openai", UpstreamID: "gpt-4o", Routable: true, CostTier: "metered"},
	}
	res := s.RecommendForCategories(hw, inv)
	if res.BudgetGB < 20 || res.BudgetGB > 36 {
		t.Fatalf("budget=%d", res.BudgetGB)
	}
	if len(res.ByCategory) != 4 {
		t.Fatalf("categories=%d", len(res.ByCategory))
	}
}

func TestRecommendSimpleUsesSmallerTier(t *testing.T) {
	s := benchmark.NewStore("")
	_ = s.Update()
	hw := benchmark.PresetMacM536
	inv := []provider.ModelInfo{
		{ProviderID: "ollama", UpstreamID: "gemma4:31b-mlx", Routable: true, CostTier: "local", Params: 31},
		{ProviderID: "ollama", UpstreamID: "qwen2.5-coder:7b-mlx", Routable: true, CostTier: "local", Params: 7},
	}
	load := []store.LoadCell{
		{Category: "chat", Complexity: "simple", Requests: 49, SharePct: 45},
		{Category: "coding", Complexity: "medium", Requests: 41, SharePct: 38},
		{Category: "coding", Complexity: "simple", Requests: 17, SharePct: 16},
	}
	res := s.RecommendWithLoad(hw, inv, load, 107)
	if len(res.ByComplexity) < 3 {
		t.Fatalf("by_complexity=%d", len(res.ByComplexity))
	}
	var chatSimple, codingMedium benchmark.CategoryRecommendation
	for _, c := range res.ByComplexity {
		if c.Category == "chat" && c.Complexity == "simple" {
			chatSimple = c
		}
		if c.Category == "coding" && c.Complexity == "medium" {
			codingMedium = c
		}
	}
	if chatSimple.DisplayName == "" {
		t.Fatalf("missing chat/simple: %+v", res.ByComplexity)
	}
	// 31B (~22GB) must not win simple tier (~10GB)
	if chatSimple.MemReqGB > chatSimple.TierBudget {
		t.Fatalf("simple over tier: %+v", chatSimple)
	}
	if chatSimple.Params >= 30 {
		t.Fatalf("simple should prefer small model, got %+v", chatSimple)
	}
	if codingMedium.DisplayName == "" {
		t.Fatalf("missing coding/medium")
	}
	if res.LoadTotal != 107 {
		t.Fatalf("load_total=%d", res.LoadTotal)
	}
}
