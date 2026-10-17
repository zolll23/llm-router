package store_test

import (
	"testing"
	"time"

	"router-app/internal/store"
)

func TestSankeySince(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UnixMilli()
	events := []store.UsageEvent{
		{TS: now, ModelID: "ollama/llama", ProviderID: "ollama", Category: "coding", Complexity: "simple", Pool: "ollama · local", HTTPStatus: 200},
		{TS: now, ModelID: "ollama/llama", ProviderID: "ollama", Category: "coding", Complexity: "simple", Pool: "ollama · local", HTTPStatus: 200},
		{TS: now, ModelID: "openai/gpt", ProviderID: "openai", Category: "reasoning", Complexity: "hard", Pool: "openai · metered", HTTPStatus: 429},
		{TS: now, ModelID: "openai/gpt", ProviderID: "openai", Category: "chat", Complexity: "trivial", Pool: "openai · metered", HTTPStatus: 500},
	}
	for _, e := range events {
		if err := db.InsertUsage(e); err != nil {
			t.Fatal(err)
		}
	}

	diag, err := db.SankeySince(7)
	if err != nil {
		t.Fatal(err)
	}
	if diag.Total != 4 {
		t.Fatalf("total=%d", diag.Total)
	}
	if len(diag.Nodes) == 0 || len(diag.Links) == 0 {
		t.Fatalf("empty diagram: %+v", diag)
	}
	var codingSimple int64
	for _, l := range diag.Links {
		if l.Source == "cat:coding" && l.Target == "cx:coding:simple" {
			codingSimple = l.Value
		}
	}
	if codingSimple != 2 {
		t.Fatalf("coding→simple want 2 got %d links=%+v", codingSimple, diag.Links)
	}
	// chat and coding must not share a complexity node
	for _, n := range diag.Nodes {
		if n.Stage == "complexity" && n.ID == "cx:simple" {
			t.Fatalf("unscoped complexity node still present: %+v", n)
		}
	}
	var codingToModel int64
	for _, l := range diag.Links {
		if l.Source == "pool:coding:ollama · local" && l.Target == "model:ollama/llama" {
			codingToModel = l.Value
		}
	}
	if codingToModel != 2 {
		t.Fatalf("coding pool→model want 2 got %d", codingToModel)
	}
}
