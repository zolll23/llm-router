package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FetchOllamaPS returns loaded model name → size in GB from GET {base}/api/ps.
func FetchOllamaPS(ctx context.Context, baseURL string) (map[string]float64, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return nil, fmt.Errorf("ollama ps: empty base URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/ps", nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama ps: HTTP %d: %s", resp.StatusCode, string(b))
	}
	var body struct {
		Models []struct {
			Name     string `json:"name"`
			Model    string `json:"model"`
			Size     int64  `json:"size"`
			SizeVRAM int64  `json:"size_vram"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(body.Models))
	for _, m := range body.Models {
		name := m.Name
		if name == "" {
			name = m.Model
		}
		if name == "" {
			continue
		}
		bytes := m.SizeVRAM
		if bytes <= 0 {
			bytes = m.Size
		}
		gb := float64(bytes) / (1024 * 1024 * 1024)
		if gb <= 0 {
			gb = 1
		}
		out[name] = gb
	}
	return out, nil
}

// ModelNameMatch reports whether a and b refer to the same Ollama tag family.
func ModelNameMatch(a, b string) bool {
	return normalizeOllamaName(a) == normalizeOllamaName(b)
}

func normalizeOllamaName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexByte(s, '@'); i >= 0 {
		s = s[:i]
	}
	return s
}
