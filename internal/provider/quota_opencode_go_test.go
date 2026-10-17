package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenCodeGoQuotaParse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zen/go/v1/usage" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("auth=%q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"usage":{"rolling":{"percent":10,"resetsAt":"2026-10-18T00:00:00Z","status":"ok"},"weekly":{"percent":14,"resetsAt":"2026-10-19T00:00:00Z"},"monthly":{"percent":10,"resetsAt":"2026-11-11T00:00:00Z"}}}`))
	}))
	defer srv.Close()
	q := NewOpenCodeGoQuota("opencode-go", srv.URL, "test-key")
	snap, err := q.FetchQuota(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Weekly == nil || snap.Weekly.UsedPercent != 14 {
		t.Fatalf("weekly=%v", snap.Weekly)
	}
	if snap.Rolling == nil || snap.Rolling.UsedPercent != 10 {
		t.Fatalf("rolling=%v", snap.Rolling)
	}
}
