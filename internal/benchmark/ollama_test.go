package benchmark

import "testing"

func TestCanonicalOllamaNameStripsFakeMLX(t *testing.T) {
	cases := map[string]string{
		"gemma2:9b-mlx":         "gemma2:9b",
		"qwen2.5-coder:14b-mlx": "qwen2.5-coder:14b",
		"gemma4:31b-mlx":        "gemma4:31b-mlx",
		"gemma4:e4b-mlx":        "gemma4:e4b-mlx",
		"llama3.1:8b":           "llama3.1:8b",
	}
	for in, want := range cases {
		if got := CanonicalOllamaName(in); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestOllamaModelExistsAllowlist(t *testing.T) {
	if !OllamaModelExists("gemma2:9b") {
		t.Fatal("gemma2:9b should be verified")
	}
	if OllamaModelExists("gemma2:9b-mlx") {
		// alias rewrites to gemma2:9b which exists — that's OK
		if CanonicalOllamaName("gemma2:9b-mlx") != "gemma2:9b" {
			t.Fatal("expected alias")
		}
	}
	if !OllamaModelExists("gemma4:31b-mlx") {
		t.Fatal("gemma4:31b-mlx should be verified")
	}
}

func TestRecommendPullNamesAreReal(t *testing.T) {
	s := NewStore("")
	_ = s.Update()
	res := s.RecommendWithLoad(PresetMacM536, nil, nil, 0)
	for _, sec := range res.Priority {
		for _, m := range sec.Models {
			if m.OllamaPull == "" {
				t.Fatalf("%s/%s model %s: empty pull", sec.Category, sec.Complexity, m.DisplayName)
			}
			if !OllamaModelExists(m.DisplayName) {
				t.Fatalf("unverified pull target %q", m.DisplayName)
			}
			if m.OllamaURL == "" {
				t.Fatalf("missing library url for %s", m.DisplayName)
			}
		}
	}
}
