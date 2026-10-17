package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type OpenAICompat struct {
	id        string
	baseURL   string
	apiKeyEnv string
	client    *http.Client
}

func NewOpenAICompat(id, baseURL, apiKeyEnv string) *OpenAICompat {
	return &OpenAICompat{
		id:        id,
		baseURL:   strings.TrimRight(baseURL, "/"),
		apiKeyEnv: apiKeyEnv,
		client:    &http.Client{Timeout: 10 * time.Minute},
	}
}

func (o *OpenAICompat) ID() string { return o.id }

func (o *OpenAICompat) authHeader() string {
	if o.apiKeyEnv == "" {
		return ""
	}
	return os.Getenv(o.apiKeyEnv)
}

type openAIModelsResp struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func (o *OpenAICompat) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if k := o.authHeader(); k != "" {
		req.Header.Set("Authorization", "Bearer "+k)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("models: %s: %s", resp.Status, string(b))
	}
	var body openAIModelsResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(body.Data))
	for _, m := range body.Data {
		out = append(out, ModelInfo{
			ProviderID:  o.id,
			UpstreamID:  m.ID,
			DisplayName: m.ID,
			Routable:    true,
			Tools:       true,
			CostTier:    "metered",
			Context:     128000,
		})
	}
	return out, nil
}

func (o *OpenAICompat) ChatCompletions(ctx context.Context, rawBody []byte, hdr http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if k := o.authHeader(); k != "" {
		req.Header.Set("Authorization", "Bearer "+k)
	}
	for k, vs := range hdr {
		if strings.EqualFold(k, "Host") || strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Authorization") {
			continue
		}
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	return o.client.Do(req)
}
