package store_test

import (
	"testing"
	"time"

	"router-app/internal/store"
)

func TestProviderLiveSince(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now()
	base := now.Add(-10 * time.Minute).UnixMilli()
	for i := 0; i < 5; i++ {
		if err := db.InsertUsage(store.UsageEvent{
			TS: base + int64(i)*60_000, ModelID: "ollama/a", ProviderID: "ollama",
			LatencyMS: 100 + i*10, PromptTokens: 1, CompletionTokens: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.InsertUsage(store.UsageEvent{
		TS: base + 120_000, ModelID: "opencode/x", ProviderID: "opencode-go",
		LatencyMS: 500, PromptTokens: 1, CompletionTokens: 1,
	}); err != nil {
		t.Fatal(err)
	}

	live, err := db.ProviderLiveSince(now.Add(-15*time.Minute).UnixMilli(), 60_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 2 {
		t.Fatalf("providers=%d %+v", len(live), live)
	}
	var ollama *store.ProviderLive
	for i := range live {
		if live[i].ProviderID == "ollama" {
			ollama = &live[i]
		}
	}
	if ollama == nil || ollama.Latency.N != 5 {
		t.Fatalf("ollama=%+v", ollama)
	}
	if ollama.Latency.P50 <= 0 {
		t.Fatalf("p50=%v", ollama.Latency)
	}
	sum := 0
	for _, b := range ollama.Series {
		sum += b.Req
	}
	if sum != 5 {
		t.Fatalf("series sum=%d", sum)
	}
}
