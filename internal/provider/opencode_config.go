package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// OpenCodeConfig discovers models declared in opencode.json (and nested openai-compatible providers).
type OpenCodeConfig struct {
	id   string
	path string
}

func NewOpenCodeConfig(id, path string) *OpenCodeConfig {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}
	return &OpenCodeConfig{id: id, path: path}
}

func (o *OpenCodeConfig) ID() string { return o.id }

func (o *OpenCodeConfig) ListModels(ctx context.Context) ([]ModelInfo, error) {
	_ = ctx
	b, err := os.ReadFile(o.path)
	if err != nil {
		return nil, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, err
	}
	// Support both "provider" (v1) and "providers" (v2).
	var provBlock json.RawMessage
	if v, ok := root["providers"]; ok {
		provBlock = v
	} else if v, ok := root["provider"]; ok {
		provBlock = v
	} else {
		return nil, nil
	}
	var providers map[string]struct {
		Models map[string]json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(provBlock, &providers); err != nil {
		return nil, err
	}
	var out []ModelInfo
	for pname, p := range providers {
		for mid := range p.Models {
			pid := "opencode_" + pname
			out = append(out, ModelInfo{
				ProviderID:  pid,
				UpstreamID:  mid,
				DisplayName: pname + "/" + mid,
				Routable:    false, // routing via nested upstreams wired later
				Tools:       true,
				CostTier:    "unknown",
				Context:     128000,
			})
			_ = o.id
		}
	}
	return out, nil
}

func (o *OpenCodeConfig) ChatCompletions(ctx context.Context, rawBody []byte, hdr http.Header) (*http.Response, error) {
	return nil, ErrNotRoutable
}
