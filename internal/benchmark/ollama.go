package benchmark

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Aliases from older catalog ids / invented "-mlx" tags → real Ollama library names.
// Verified against registry.ollama.ai (library manifests).
var ollamaAliases = map[string]string{
	"gemma2:9b-mlx":         "gemma2:9b",
	"gemma2:2b-mlx":         "gemma2:2b",
	"qwen2.5-coder:7b-mlx":  "qwen2.5-coder:7b",
	"qwen2.5-coder:14b-mlx": "qwen2.5-coder:14b",
	"qwen2.5-coder:32b-mlx": "qwen2.5-coder:32b",
	"qwen2.5:14b-mlx":       "qwen2.5:14b",
	"llama3.1:8b-mlx":       "llama3.1:8b",
	"llama3.1:70b-mlx":      "llama3.1:70b",
	"llama3.2:3b-mlx":       "llama3.2:3b",
	"gemma-4-2b":            "gemma4:e2b-mlx",
	"gemma-4-9b":            "gemma4:e4b-mlx",
}

// Known-good pull names (skip network). gemma4-*-mlx are the only MLX tags in library today.
var ollamaVerified = map[string]bool{
	"gemma2:2b":         true,
	"gemma2:9b":         true,
	"gemma2:27b":        true,
	"gemma4:e2b-mlx":    true,
	"gemma4:e4b-mlx":    true,
	"gemma4:12b-mlx":    true,
	"gemma4:26b-mlx":    true,
	"gemma4:31b-mlx":    true,
	"gemma4:e2b":        true,
	"gemma4:e4b":        true,
	"gemma4:12b":        true,
	"gemma4:26b":        true,
	"gemma4:31b":        true,
	"qwen2.5-coder:7b":  true,
	"qwen2.5-coder:14b": true,
	"qwen2.5-coder:32b": true,
	"qwen2.5:14b":       true,
	"llama3.1:8b":       true,
	"llama3.1:70b":      true,
	"llama3.2:3b":       true,
}

var (
	registryOnce   sync.Once
	registryClient *http.Client
	registryMu     sync.Mutex
	registryCache  = map[string]bool{}
)

func registryHTTP() *http.Client {
	registryOnce.Do(func() {
		registryClient = &http.Client{Timeout: 3 * time.Second}
	})
	return registryClient
}

// CanonicalOllamaName maps invented / legacy ids to real Ollama pull names.
func CanonicalOllamaName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	if v, ok := ollamaAliases[strings.ToLower(name)]; ok {
		return v
	}
	family, tag, hasTag := strings.Cut(name, ":")
	if !hasTag {
		return name
	}
	family = strings.TrimSpace(family)
	tag = strings.TrimSpace(tag)
	lowFam := strings.ToLower(family)
	lowTag := strings.ToLower(tag)
	// Only gemma4 publishes MLX tags on ollama.com/library today.
	if strings.Contains(lowTag, "mlx") && lowFam != "gemma4" {
		tag = stripMLXFromTag(tag)
		if tag == "" {
			return family
		}
		return family + ":" + tag
	}
	return family + ":" + tag
}

func stripMLXFromTag(tag string) string {
	t := tag
	repl := []string{"-mlx-bf16", "-mlx-fp16", "-mxfp8", "-nvfp4", "-mlx"}
	low := strings.ToLower(t)
	for _, s := range repl {
		if i := strings.Index(low, s); i >= 0 {
			t = t[:i] + t[i+len(s):]
			low = strings.ToLower(t)
		}
	}
	return strings.Trim(t, "-_")
}

func splitOllamaRef(name string) (family, tag string) {
	name = CanonicalOllamaName(name)
	family, tag, ok := strings.Cut(name, ":")
	if !ok {
		return name, "latest"
	}
	if tag == "" {
		tag = "latest"
	}
	return family, tag
}

func ollamaLibraryURL(name string) string {
	family, _ := splitOllamaRef(name)
	if family == "" {
		return "https://ollama.com/library"
	}
	return "https://ollama.com/library/" + url.PathEscape(family)
}

func ollamaPullCommand(name string) string {
	n := CanonicalOllamaName(name)
	if n == "" {
		return ""
	}
	return "ollama pull " + n
}

// OllamaModelExists reports whether library/registry has this model:tag.
// Uses a static allowlist first, then a short registry HEAD with process cache.
func OllamaModelExists(name string) bool {
	n := CanonicalOllamaName(name)
	if n == "" {
		return false
	}
	if ollamaVerified[strings.ToLower(n)] {
		return true
	}
	registryMu.Lock()
	if v, ok := registryCache[n]; ok {
		registryMu.Unlock()
		return v
	}
	registryMu.Unlock()

	family, tag := splitOllamaRef(n)
	u := "https://registry.ollama.ai/v2/library/" + url.PathEscape(family) + "/manifests/" + url.PathEscape(tag)
	req, err := http.NewRequest(http.MethodHead, u, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	resp, err := registryHTTP().Do(req)
	ok := false
	if err == nil {
		_ = resp.Body.Close()
		ok = resp.StatusCode == http.StatusOK
	}
	registryMu.Lock()
	registryCache[n] = ok
	registryMu.Unlock()
	return ok
}
