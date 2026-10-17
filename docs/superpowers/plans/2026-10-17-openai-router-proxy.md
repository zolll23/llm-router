# OpenAI Router Proxy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a Go OpenAI-compatible router proxy that OpenCode selects as `auto`, discovers models from configured providers (namespaced IDs), classifies with Nimble, routes local-first with sticky sessions, and exposes Usage/Settings/Quality UI per the design spec.

**Architecture:** Single binary: HTTP API → Inventory (provider adapters) → Classifier → Scorer → Sticky → Upstream proxy → SQLite usage/eval. Virtual model `auto`. Spec: `docs/superpowers/specs/2026-10-17-openai-router-proxy-design.md`.

**Tech Stack:** Go 1.22+, stdlib `net/http`, `gopkg.in/yaml.v3`, `modernc.org/sqlite`, `embed` for UI.

## Global Constraints

- OpenAI surface v1: only `/v1/models` + `/v1/chat/completions` + `/health` (no completions/embeddings/responses).
- Canonical backend ID: `provider_id/upstream_id` — never merge stats across providers.
- Default listen `127.0.0.1:8080`; non-loopback requires token.
- Classifier default: Nimble via Ollama `/v1/systemone`; heuristic fallback.
- Always inject `stream_options.include_usage` on streamed upstream requests.
- UI Russian; code/config English.
- SQLite under `data_dir` (default `~/.local/share/router`).
- TDD: failing test → implement → pass → commit per task.
- YAGNI: Cursor chat routing and AI-judge are later tasks; discover-only Cursor first.

---

## File Structure

```
cmd/router/main.go
internal/config/config.go
internal/config/config_test.go
internal/provider/provider.go          # Provider interface, ModelInfo, CanonicalID
internal/provider/registry.go          # Inventory sync
internal/provider/ollama.go
internal/provider/openai_compat.go
internal/provider/opencode_config.go
internal/provider/cursor_cli.go
internal/provider/canonical_test.go
internal/classifier/classifier.go
internal/classifier/nimble.go
internal/classifier/heuristic.go
internal/classifier/nimble_test.go
internal/router/decide.go
internal/router/scorer.go
internal/router/sticky.go
internal/router/decide_test.go
internal/proxy/chat.go                 # SSE + usage harvest hooks
internal/store/db.go
internal/store/usage.go
internal/store/routing.go
internal/api/server.go
internal/api/auth.go
internal/api/openai.go
internal/api/admin.go
internal/api/openai_test.go
web/index.html
web/app.js
web/styles.css
configs/config.example.yaml
go.mod
README.md
```

---

### Task 1: Module bootstrap + health

**Files:**
- Create: `go.mod`, `cmd/router/main.go`, `internal/config/config.go`, `internal/config/config_test.go`, `configs/config.example.yaml`, `README.md`

**Interfaces:**
- Produces: `config.Load(path string) (*Config, error)` with fields `Listen`, `DataDir`, `VirtualModel`, `AdminToken`, `APIToken`, `Providers []ProviderConfig`

- [ ] **Step 1: Write failing test for default config**

```go
// internal/config/config_test.go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"router/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("providers: []\n"), 0o644); err != nil {
		t.fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:8080" {
		t.Fatalf("listen: %q", cfg.Listen)
	}
	if cfg.VirtualModel != "auto" {
		t.Fatalf("virtual: %q", cfg.VirtualModel)
	}
}
```

- [ ] **Step 2: Run test — expect FAIL**

Run: `cd /Users/pahomov/work/router && go test ./internal/config/ -v`  
Expected: fail (package/module missing)

- [ ] **Step 3: Implement minimal config + main**

```go
// go.mod
module router

go 1.22

require gopkg.in/yaml.v3 v3.0.1
```

```go
// internal/config/config.go
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type ProviderConfig struct {
	ID         string `yaml:"id"`
	Type       string `yaml:"type"`
	BaseURL    string `yaml:"base_url"`
	APIKeyEnv  string `yaml:"api_key_env"`
	ConfigPath string `yaml:"config_path"`
	Command    string `yaml:"command"`
}

type Config struct {
	Listen       string           `yaml:"listen"`
	DataDir      string           `yaml:"data_dir"`
	VirtualModel string           `yaml:"virtual_model"`
	AdminToken   string           `yaml:"admin_token"`
	APIToken     string           `yaml:"api_token"`
	Providers    []ProviderConfig `yaml:"providers"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		Listen:       "127.0.0.1:8080",
		VirtualModel: "auto",
		DataDir:      filepath.Join(os.Getenv("HOME"), ".local/share/router"),
	}
	if err := yaml.Unmarshal(b, cfg); err != nil {
		return nil, err
	}
	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:8080"
	}
	if cfg.VirtualModel == "" {
		cfg.VirtualModel = "auto"
	}
	return cfg, nil
}
```

```go
// cmd/router/main.go
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"

	"router/internal/config"
)

func main() {
	cfgPath := flag.String("config", "configs/config.example.yaml", "config path")
	flag.Parse()
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	log.Printf("listening on %s", cfg.Listen)
	log.Fatal(http.ListenAndServe(cfg.Listen, mux))
}
```

- [ ] **Step 4: Run tests — expect PASS**

Run: `go mod tidy && go test ./internal/config/ -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum cmd/ internal/config/ configs/config.example.yaml README.md
git commit -m "feat: bootstrap router config and health endpoint"
```

---

### Task 2: Canonical IDs + Provider interface

**Files:**
- Create: `internal/provider/provider.go`, `internal/provider/canonical_test.go`

**Interfaces:**
- Produces:
  - `type ModelInfo struct { ProviderID, UpstreamID, DisplayName string; Routable bool; Tools, Vision bool; Context int; CostTier string }`
  - `func CanonicalID(providerID, upstreamID string) string`
  - `type Provider interface { ID() string; ListModels(ctx context.Context) ([]ModelInfo, error); ChatCompletions(ctx context.Context, rawBody []byte, hdr http.Header) (*http.Response, error) }`
  - ChatCompletions may return `ErrNotRoutable`

- [ ] **Step 1: Failing test**

```go
func TestCanonicalID(t *testing.T) {
	got := provider.CanonicalID("ollama", "qwen3-coder")
	if got != "ollama/qwen3-coder" {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Run — FAIL**

Run: `go test ./internal/provider/ -v`

- [ ] **Step 3: Implement `provider.go`**

```go
package provider

import (
	"context"
	"errors"
	"net/http"
)

var ErrNotRoutable = errors.New("provider cannot chat")

type ModelInfo struct {
	ProviderID  string
	UpstreamID  string
	DisplayName string
	Routable    bool
	Tools       bool
	Vision      bool
	Context     int
	CostTier    string // local | metered | unknown
}

func CanonicalID(providerID, upstreamID string) string {
	return providerID + "/" + upstreamID
}

func (m ModelInfo) ID() string { return CanonicalID(m.ProviderID, m.UpstreamID) }

type Provider interface {
	ID() string
	ListModels(ctx context.Context) ([]ModelInfo, error)
	ChatCompletions(ctx context.Context, rawBody []byte, hdr http.Header) (*http.Response, error)
}
```

- [ ] **Step 4: PASS + Commit**

```bash
git commit -m "feat: provider interface and canonical model IDs"
```

---

### Task 3: Ollama adapter ListModels + Chat

**Files:**
- Create: `internal/provider/ollama.go`, `internal/provider/ollama_test.go`

**Interfaces:**
- Consumes: `Provider`, `ModelInfo`
- Produces: `func NewOllama(id, baseURL string) *Ollama`

- [ ] **Step 1: httptest test for ListModels parsing `{"models":[{"name":"llama3.2"}]}` → `ollama/llama3.2`, CostTier local, Routable true**

- [ ] **Step 2: FAIL then implement ListModels via `GET {base}/api/tags` and Chat via `POST {base}/v1/chat/completions` (forward body after rewriting model field to upstream id)**

- [ ] **Step 3: PASS + commit** `feat: ollama provider adapter`

---

### Task 4: Registry sync + GET /api/inventory + GET /v1/models

**Files:**
- Create: `internal/provider/registry.go`, `internal/api/server.go`, `internal/api/openai.go`, `internal/api/openai_test.go`
- Modify: `cmd/router/main.go`

**Interfaces:**
- Produces: `type Registry struct`; `func (r *Registry) Sync(ctx) error`; `func (r *Registry) Models() []ModelInfo`; `func (r *Registry) Get(canonicalID string) (ModelInfo, Provider, bool)`

- [ ] **Step 1: Test `/v1/models` returns virtual `auto` plus synced ollama models with ids `provider/name`**

- [ ] **Step 2: Implement Registry + wire mux; background sync every 60s**

- [ ] **Step 3: PASS + commit** `feat: inventory registry and /v1/models`

---

### Task 5: Naive chat proxy for `auto` → first local routable model

**Files:**
- Create: `internal/proxy/chat.go`, `internal/proxy/chat_test.go`
- Modify: `internal/api/openai.go`

**Interfaces:**
- Produces: `func ProxyChat(ctx, reg, virtualModel, body []byte, hdr) (*http.Response, chosen ModelInfo, error)` — for this task always pick first `Routable && CostTier==local` else first routable

- [ ] **Step 1: httptest upstream receives rewritten model name; client gets 200 JSON**

- [ ] **Step 2: Implement non-stream proxy first; pass through status/body; set `X-Router-Model`**

- [ ] **Step 3: PASS + commit** `feat: proxy chat completions to upstream`

---

### Task 6: SSE streaming + include_usage injection

**Files:**
- Modify: `internal/proxy/chat.go`, tests

- [ ] **Step 1: Test that when request JSON has `"stream":true`, outbound body includes `stream_options.include_usage=true`**

- [ ] **Step 2: Implement JSON patch + `io.Copy` of response body with `Content-Type` flush for SSE**

- [ ] **Step 3: Commit** `feat: sse passthrough with usage injection`

---

### Task 7: Nimble classifier + heuristic fallback

**Files:**
- Create: `internal/classifier/*.go` + tests

**Interfaces:**
- Produces:
```go
type Result struct {
	Complexity string  // trivial|simple|medium|hard
	NeedsTools bool
	Confidence float64
	Source     string  // nimble|heuristic
}
type Classifier interface {
	Classify(ctx context.Context, in Input) (Result, error)
}
type Input struct {
	UserText      string
	ToolsPresent  bool
	ApproxTokens  int
	MessageCount  int
}
```

- [ ] **Step 1: Mock systemone JSON → Complexity medium**

- [ ] **Step 2: Implement Nimble client `POST {ollama}/v1/systemone`; heuristic on error**

- [ ] **Step 3: Commit** `feat: nimble classifier with heuristic fallback`

---

### Task 8: Scorer + sticky + decide

**Files:**
- Create: `internal/router/scorer.go`, `sticky.go`, `decide.go`, `decide_test.go`

**Interfaces:**
```go
func Decide(ctx context.Context, in DecideInput) (DecideOutput, error)
type DecideInput struct {
	Class      classifier.Result
	Models     []provider.ModelInfo
	SessionKey string
	Sticky     *StickyStore
	Now        time.Time
}
type DecideOutput struct {
	Model  provider.ModelInfo
	Reason string
}
```

- [ ] **Step 1: Test simple+local preferred; sticky returns same model on second Decide**

- [ ] **Step 2: Implement filter routable → score local-first → sticky TTL 30m**

- [ ] **Step 3: Wire into ProxyChat; commit** `feat: local-first scoring and sticky sessions`

---

### Task 9: SQLite usage + harvest

**Files:**
- Create: `internal/store/db.go`, `usage.go`, tests
- Modify: `proxy/chat.go` to record events (parse usage from JSON or last SSE chunk)

- [ ] **Step 1: Test Insert + Overview aggregates for one model day window**

- [ ] **Step 2: Schema `usage_events`; Overview API handler**

- [ ] **Step 3: Commit** `feat: sqlite usage accounting`

---

### Task 10: OpenAI-compatible + OpenCode + Cursor list adapters

**Files:**
- Create: `openai_compat.go`, `opencode_config.go`, `cursor_cli.go` + tests
- Cursor: ListModels via `exec.CommandContext(command, "--list-models")`; Chat returns `ErrNotRoutable`

- [ ] **Step 1: Unit tests with fake HTTP / fake command output**

- [ ] **Step 2: Implement; factory `provider.NewFromConfig(ProviderConfig)`**

- [ ] **Step 3: Commit** `feat: openai, opencode, and cursor inventory adapters`

---

### Task 11: Auth for non-loopback

**Files:**
- Create: `internal/api/auth.go`, test

- [ ] **Step 1: Test loopback skips auth; 0.0.0.0 requires Bearer**

- [ ] **Step 2: Middleware; commit** `feat: token auth when not binding loopback`

---

### Task 12: Embedded Usage + Settings + Quality (online) UI

**Files:**
- Create: `web/*`, `internal/api/admin.go`, embed in `server.go`

- [ ] **Step 1: Manual checklist — three tabs load; overview JSON renders; settings PUT persists YAML or settings table**

- [ ] **Step 2: Minimal vanilla JS UI (Russian labels); Quality shows escalation/local share from `routing_events`**

- [ ] **Step 3: Commit** `feat: embedded usage settings and quality UI`

---

### Task 13: routing_events online signals

**Files:**
- Modify: store + decide/proxy to set escalated/sticky_break/user_override/local_success

- [ ] **Step 1: Test escalation flag when session upgrades tier after tool failure**

- [ ] **Step 2: Implement; commit** `feat: online routing quality signals`

---

### Task 14: AI-judge job + confusion + GLiNER export (phase 2)

**Files:**
- Create: `internal/eval/judge.go`, `scheduler.go`, `export.go`, admin routes

- [ ] **Step 1: Mock judge labels → confusion matrix 2x2**

- [ ] **Step 2: Periodic sampler; export JSONL path under `data_dir/finetune/`**

- [ ] **Step 3: Commit** `feat: ai-judge eval loop and finetune export`

---

### Task 15: README OpenCode snippet + example config

**Files:**
- Modify: `README.md`, `configs/config.example.yaml`

- [ ] Document OpenCode provider block pointing at `http://127.0.0.1:8080/v1`, model `auto`, tools true, context 128000

- [ ] Commit: `docs: OpenCode integration and example config`

---

## Spec coverage check

| Spec section | Tasks |
|--------------|-------|
| §3 Architecture | 1–8 |
| §4 Providers / namespaced IDs | 2–4, 10 |
| §5 Pipeline classify/score/sticky | 7–8 |
| §6 SQLite usage | 9, 13 |
| §7 Quality phase1/2 | 12–14 |
| §8 UI/security | 11–12 |
| §9 API surface | 4–6, 9, 12 |
| Cursor discover-only then probe | 10, (14 optional chat later) |
| No completions/embeddings/responses | Global constraint |

## Placeholder scan

None intentional; Task 12 UI is behavioral checklist + minimal JS rather than full CSS dump — implementer writes lean vanilla UI matching spec tabs.

---

## Execution

Plan saved to `docs/superpowers/plans/2026-10-17-openai-router-proxy.md`.

**Two execution options:**

1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks  
2. **Inline Execution** — this session with executing-plans checkpoints  

Given your «приступай», defaulting to **Inline Execution** unless you say otherwise.
