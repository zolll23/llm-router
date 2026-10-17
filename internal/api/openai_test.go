package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"router-app/internal/api"
	"router-app/internal/config"
	"router-app/internal/provider"
	"router-app/internal/router"
	"router-app/internal/store"
)

type stubProvider struct {
	id     string
	models []provider.ModelInfo
}

func (s *stubProvider) ID() string { return s.id }
func (s *stubProvider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return s.models, nil
}
func (s *stubProvider) ChatCompletions(ctx context.Context, rawBody []byte, hdr http.Header) (*http.Response, error) {
	return nil, provider.ErrNotRoutable
}

func TestV1ModelsIncludesVirtualAndInventory(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&stubProvider{
		id: "ollama",
		models: []provider.ModelInfo{{
			ProviderID: "ollama", UpstreamID: "llama3.2", DisplayName: "llama3.2",
			Routable: true, CostTier: "local",
		}},
	})
	if err := reg.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	settings, err := api.NewSettingsStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := &api.Server{
		Cfg:      &config.Config{Listen: "127.0.0.1:8080", VirtualModel: "auto"},
		Reg:      reg,
		DB:       db,
		Sticky:   router.NewStickyStore(time.Hour),
		Settings: settings,
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, d := range body.Data {
		ids[d.ID] = true
	}
	if !ids["auto"] || !ids["ollama/llama3.2"] {
		t.Fatalf("ids %#v", ids)
	}
}
