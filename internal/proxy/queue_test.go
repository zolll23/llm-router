package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"router-app/internal/classifier"
	"router-app/internal/provider"
	"router-app/internal/router"
	"router-app/internal/scheduler"
)

type stubProv struct {
	id   string
	mods []provider.ModelInfo
}

func (s *stubProv) ID() string { return s.id }
func (s *stubProv) ListModels(context.Context) ([]provider.ModelInfo, error) {
	return s.mods, nil
}
func (s *stubProv) ChatCompletions(ctx context.Context, raw []byte, hdr http.Header) (*http.Response, error) {
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"ok"}}]}`)),
	}, nil
}

type fixedClass struct{ r classifier.Result }

func (f fixedClass) Classify(context.Context, classifier.Input) (classifier.Result, error) {
	return f.r, nil
}

func TestProxyQueueOverflow429(t *testing.T) {
	reg := provider.NewRegistry()
	local := &stubProv{id: "ollama", mods: []provider.ModelInfo{
		{ProviderID: "ollama", UpstreamID: "a:7b", Routable: true, Tools: true, CostTier: "local", Params: 7},
		{ProviderID: "ollama", UpstreamID: "b:14b", Routable: true, Tools: true, CostTier: "local", Params: 14},
	}}
	reg.Register(local)
	_ = reg.Sync(context.Background())

	sched := scheduler.NewRegistry()
	loaded := map[string]float64{"a:7b": 6}
	s := scheduler.NewLocal(8, func(context.Context) (map[string]float64, error) {
		return loaded, nil
	})
	sched.Register("ollama", s)

	// Occupy the only slot with a:7b
	_, rel, err := s.Acquire(context.Background(), "a:7b", 6, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	// Fill wait queue with one waiter wanting b
	ctxWait, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_, _, _ = s.Acquire(ctxWait, "b:14b", 10, 1)
	}()
	time.Sleep(40 * time.Millisecond)

	body, _ := json.Marshal(map[string]any{
		"model": "auto",
		"messages": []map[string]string{
			{"role": "user", "content": "hi"},
		},
	})
	deps := Deps{
		Reg:        reg,
		Classifier: fixedClass{r: classifier.Result{Category: "chat", Complexity: "simple", Source: "test"}},
		Sticky:     router.NewStickyStore(time.Minute),
		Schedulers: sched,
		Settings: func() RoutingSettings {
			return RoutingSettings{EscalateMin: "hard", LocalQueueMax: 1, LocalQueueRedirect: false, KeepAlive: "1m"}
		},
	}
	res, err := ProxyChat(context.Background(), deps, body, http.Header{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Response.StatusCode != 429 {
		t.Fatalf("status=%d reason=%s", res.Response.StatusCode, res.Reason)
	}
	if res.Reason != "queue_overflow" {
		t.Fatalf("reason=%s", res.Reason)
	}
}
