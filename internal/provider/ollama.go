package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Ollama struct {
	id      string
	baseURL string
	client  *http.Client
}

func NewOllama(id, baseURL string) *Ollama {
	return &Ollama{
		id:      id,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 10 * time.Minute},
	}
}

func (o *Ollama) ID() string { return o.id }

type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

func (o *Ollama) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama tags: %s: %s", resp.Status, string(b))
	}
	var tags ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(tags.Models))
	for _, m := range tags.Models {
		tier := "local"
		if strings.Contains(m.Name, ":cloud") || strings.HasSuffix(m.Name, "-cloud") {
			tier = "metered"
		}
		out = append(out, ModelInfo{
			ProviderID:  o.id,
			UpstreamID:  m.Name,
			DisplayName: m.Name,
			Routable:    true,
			Tools:       true,
			CostTier:    tier,
			Context:     128000,
		})
	}
	return out, nil
}

func (o *Ollama) ChatCompletions(ctx context.Context, rawBody []byte, hdr http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/v1/chat/completions", bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, vs := range hdr {
		if strings.EqualFold(k, "Host") || strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	return o.client.Do(req)
}
