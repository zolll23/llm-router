package classifier_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"router-app/internal/classifier"
)

func TestNimbleClassify(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" {
			t.Fatalf("path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{
				"category":    map[string]any{"type": "choice", "choice": "coding", "confidence": 0.85},
				"complexity":  map[string]any{"type": "choice", "choice": "medium", "confidence": 0.9},
				"needs_tools": map[string]any{"type": "noul", "noul": 0.8},
			},
		})
	}))
	defer srv.Close()

	n := classifier.NewNimble(srv.URL, "nimble", &classifier.Heuristic{})
	res, err := n.Classify(context.Background(), classifier.Input{UserText: "implement feature"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Complexity != "medium" || res.Category != "coding" || !res.NeedsTools || res.Source != "nimble" {
		t.Fatalf("%+v", res)
	}
}

func TestHeuristicFallback(t *testing.T) {
	n := classifier.NewNimble("http://127.0.0.1:1", "nimble", &classifier.Heuristic{})
	res, err := n.Classify(context.Background(), classifier.Input{UserText: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != "heuristic" {
		t.Fatalf("%+v", res)
	}
}
