package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"router-app/internal/api"
	"router-app/internal/config"
)

func TestUIHandlerServesIndex(t *testing.T) {
	h, err := api.UIHandler()
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET / -> %d loc=%q body=%q", rr.Code, rr.Header().Get("Location"), rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") && !strings.Contains(body, "<html") {
		snip := body
		if len(snip) > 80 {
			snip = snip[:80]
		}
		t.Fatalf("expected html, got %q", snip)
	}
}

func TestServerServesUIRoot(t *testing.T) {
	ui, err := api.UIHandler()
	if err != nil {
		t.Fatal(err)
	}
	s := &api.Server{
		Cfg: &config.Config{Listen: "127.0.0.1:8080", VirtualModel: "auto"},
		UI:  ui,
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET / via Server -> %d loc=%q", rr.Code, rr.Header().Get("Location"))
	}
}

func TestServerServesSPARoutes(t *testing.T) {
	ui, err := api.UIHandler()
	if err != nil {
		t.Fatal(err)
	}
	s := &api.Server{
		Cfg: &config.Config{Listen: "127.0.0.1:8080", VirtualModel: "auto"},
		UI:  ui,
	}
	for _, path := range []string{"/usage", "/quality", "/models", "/settings"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		s.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("GET %s -> %d", path, rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "data-tab") {
			t.Fatalf("GET %s: expected SPA html", path)
		}
	}
}
