package store_test

import (
	"testing"
	"time"

	"router-app/internal/store"
)

func TestUsageOverview(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	if err := db.InsertUsage(store.UsageEvent{
		TS: now.UnixMilli(), ModelID: "ollama/llama", ProviderID: "ollama",
		PromptTokens: 10, CompletionTokens: 5, LatencyMS: 100,
	}); err != nil {
		t.Fatal(err)
	}
	ov, err := db.Overview(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(ov.Day) != 1 || ov.Day[0].Requests != 1 {
		t.Fatalf("%+v", ov.Day)
	}
}
