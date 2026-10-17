package classifier_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"router-app/internal/classifier"
)

func TestGLiNERClassifyOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/classify" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body["text"] == "" {
			t.Fatal("expected text")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": map[string]any{
				"category": map[string]any{
					"label":      "coding",
					"confidence": 0.88,
				},
				"complexity": map[string]any{
					"label":      "medium",
					"confidence": 0.91,
					"probabilities": map[string]float64{
						"trivial": 0.01, "simple": 0.08, "medium": 0.91, "hard": 0.0,
					},
				},
				"needs_tools": map[string]any{
					"label":      "yes",
					"confidence": 0.8,
				},
			},
		})
	}))
	defer srv.Close()

	g := classifier.NewGLiNER(srv.URL, &classifier.Heuristic{})
	res, err := g.Classify(context.Background(), classifier.Input{
		UserText:     "implement auth middleware and tests",
		ToolsPresent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Complexity != "medium" || res.Category != "coding" || !res.NeedsTools || res.Source != "gliner" {
		t.Fatalf("got %+v", res)
	}
	if res.Confidence < 0.9 {
		t.Fatalf("confidence %v", res.Confidence)
	}
}

func TestGLiNERFallsBackOnError(t *testing.T) {
	g := classifier.NewGLiNER("http://127.0.0.1:1", &classifier.Heuristic{})
	res, err := g.Classify(context.Background(), classifier.Input{UserText: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != "heuristic" {
		t.Fatalf("want heuristic fallback, got %+v", res)
	}
}
