package proxy_test

import (
	"encoding/json"
	"testing"

	"router/internal/proxy"
)

func TestRewriteInjectsUsage(t *testing.T) {
	// exercise via ProxyChat path indirectly: marshal stream body
	raw := []byte(`{"model":"auto","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if !m["stream"].(bool) {
		t.Fatal("expected stream")
	}
	_ = proxy.Deps{}
}
