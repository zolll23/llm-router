package benchmark

import (
	"net/url"
	"strings"
	"testing"
)

func TestHFSearchQuery(t *testing.T) {
	cases := map[string]string{
		"gemma2:9b-mlx":           "gemma-2 9b mlx",
		"qwen2.5-coder:14b-mlx":   "qwen2.5-coder 14b mlx",
		"qwen2.5-coder:7b-mlx":    "qwen2.5-coder 7b mlx",
		"llama3.1:8b":             "llama-3.1 8b",
		"mistral:7b":              "mistral 7b",
		"gemma2:9b":               "gemma-2 9b",
	}
	for in, want := range cases {
		got := hfSearchQuery(in)
		if got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestBenchmarkSearchURLUsesNormalizedQuery(t *testing.T) {
	u := benchmarkSearchURL("gemma2:9b-mlx")
	if strings.Contains(u, "gemma2%3A9b") || strings.Contains(u, "gemma2:9b") {
		t.Fatalf("raw ollama id leaked into URL: %s", u)
	}
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query().Get("search")
	if q != "gemma-2 9b mlx" {
		t.Fatalf("search=%q", q)
	}
}
