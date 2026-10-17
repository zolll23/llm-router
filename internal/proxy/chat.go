package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"router/internal/provider"
)

type ChatResult struct {
	Response *http.Response
	Model    provider.ModelInfo
	Reason   string
}

// PickFirstLocal chooses first local routable model, else first routable.
func PickFirstLocal(models []provider.ModelInfo) (provider.ModelInfo, bool) {
	var firstRoutable provider.ModelInfo
	found := false
	for _, m := range models {
		if !m.Routable {
			continue
		}
		if !found {
			firstRoutable = m
			found = true
		}
		if m.CostTier == "local" {
			return m, true
		}
	}
	return firstRoutable, found
}

func rewriteModel(raw []byte, upstreamID string) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	m["model"] = upstreamID
	if stream, ok := m["stream"].(bool); ok && stream {
		so, _ := m["stream_options"].(map[string]any)
		if so == nil {
			so = map[string]any{}
		}
		so["include_usage"] = true
		m["stream_options"] = so
	}
	return json.Marshal(m)
}

func ProxyChat(ctx context.Context, reg *provider.Registry, body []byte, hdr http.Header) (*ChatResult, error) {
	models := reg.Models()
	chosen, ok := PickFirstLocal(models)
	if !ok {
		return nil, fmt.Errorf("no routable models in inventory")
	}
	_, prov, ok := reg.Get(chosen.ID())
	if !ok {
		return nil, fmt.Errorf("provider missing for %s", chosen.ID())
	}
	rewritten, err := rewriteModel(body, chosen.UpstreamID)
	if err != nil {
		return nil, err
	}
	resp, err := prov.ChatCompletions(ctx, rewritten, hdr)
	if err != nil {
		return nil, err
	}
	return &ChatResult{Response: resp, Model: chosen, Reason: "first-local"}, nil
}

func CopyResponse(w http.ResponseWriter, resp *http.Response, model provider.ModelInfo, reason string) error {
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("X-Router-Model", model.ID())
	w.Header().Set("X-Router-Provider", model.ProviderID)
	w.Header().Set("X-Router-Reason", reason)
	w.WriteHeader(resp.StatusCode)
	_, err := io.Copy(w, resp.Body)
	return err
}
