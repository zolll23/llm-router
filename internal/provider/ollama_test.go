package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"router-app/internal/provider"
)

func TestOllamaListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Fatalf("path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]string{{"name": "llama3.2"}},
		})
	}))
	defer srv.Close()

	o := provider.NewOllama("ollama", srv.URL)
	models, err := o.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("len %d", len(models))
	}
	if models[0].ID() != "ollama/llama3.2" {
		t.Fatalf("id %q", models[0].ID())
	}
	if models[0].CostTier != "local" || !models[0].Routable {
		t.Fatalf("model %+v", models[0])
	}
}
