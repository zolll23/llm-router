package provider

import "testing"

func TestPerfStoreObserveApply(t *testing.T) {
	p := NewPerfStore()
	p.Observe("ollama/a", 100, 50)
	p.Observe("ollama/a", 300, 50)
	m := ModelInfo{ProviderID: "ollama", UpstreamID: "a"}
	p.Apply(&m)
	if m.Metrics["latency_p50"] < 100 || m.Metrics["latency_p50"] > 300 {
		t.Fatalf("p50=%v", m.Metrics["latency_p50"])
	}
	if m.Metrics["tps"] <= 0 {
		t.Fatalf("tps=%v", m.Metrics["tps"])
	}
}

func TestEstimateParamsFromName(t *testing.T) {
	if EstimateParamsFromName("qwen2.5-coder:14b") != 14 {
		t.Fatal()
	}
	if EstimateParamsFromName("gemma4:e4b-mlx") != 4 {
		t.Fatal()
	}
}
