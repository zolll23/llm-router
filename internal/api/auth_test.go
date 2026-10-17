package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"router/internal/api"
	"router/internal/config"
	"router/internal/provider"
	"router/internal/router"
	"router/internal/store"
)

func TestAuthRequiredOffLoopback(t *testing.T) {
	dir := t.TempDir()
	db, _ := store.Open(dir)
	t.Cleanup(func() { _ = db.Close() })
	settings, _ := api.NewSettingsStore(dir)
	srv := &api.Server{
		Cfg:      &config.Config{Listen: "0.0.0.0:8080", AdminToken: "secret", VirtualModel: "auto"},
		Reg:      provider.NewRegistry(),
		DB:       db,
		Sticky:   router.NewStickyStore(time.Hour),
		Settings: settings,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rr.Code)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req2.Header.Set("Authorization", "Bearer secret")
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("got %d %s", rr2.Code, rr2.Body.String())
	}
}
