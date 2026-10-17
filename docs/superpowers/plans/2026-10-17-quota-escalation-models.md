# Quota Escalation + Models Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Эскалация сложных запросов на внешние модели при запасе по лимитам (OpenCode Go / Ollama Cloud), страница «Модели» с inventory+квотами, «Рекомендации» вместо старых Моделей, живые замеры в скорере.

**Architecture:** Опциональный `QuotaProvider` + `QuotaStore` (poll ~60s); `PerformanceStore` из `usage_events` подмешивает Metrics перед Decide; scorer получает бонус external при escalate+headroom; proxy на 429 ставит cooldown и один retry на лучшую локальную. UI: `/models` inventory, `/recommendations` бывший recommend UI, settings — пороги.

**Tech Stack:** Go, существующие `provider` / `router` / `proxy` / `store` / embed UI (Bootstrap), httptest tests.

**Spec:** `docs/superpowers/specs/2026-10-17-quota-escalation-models-design.md`

## Global Constraints

- Эскалация только если `complexity >= escalate_min` (default `hard`) и headroom OK.
- Headroom: все окна с данными `used% <` порогов (default 85/90/95); без успешного FetchQuota у адаптера — эскалация на провайдер выключена; `quota: none` — headroom=true до 429.
- На 429: cooldown + один retry на лучшую локальную; без цепочки нескольких внешних.
- UI copy/paths: Рекомендации=`/recommendations`, Модели=`/models`.
- Коммиты — только если пользователь явно попросил (иначе пропускать Step Commit).

## File map

| File | Responsibility |
|------|----------------|
| `internal/provider/quota.go` | `QuotaSnapshot`, `QuotaWindow`, `QuotaProvider` interface |
| `internal/provider/quota_opencode_go.go` | Fetch `zen/go/v1/usage` |
| `internal/provider/quota_ollama_balance.go` | Fetch ollama.com `/api/balance` |
| `internal/provider/quota_store.go` | Cache, cooldown, `HeadroomOK`, periodic refresh |
| `internal/provider/perf_store.go` | Sliding latency/tps from DB or in-memory updates |
| `internal/provider/registry.go` | Enrich models with Params + Metrics on sync/read |
| `internal/provider/openai_compat.go` / config | Optional `Quota` field wiring |
| `internal/config/config.go` | `ProviderConfig.Quota` |
| `internal/router/scorer.go` / `decide.go` | Escalate bias, sticky break on pool mismatch |
| `internal/proxy/chat.go` | Inject deps, 429 retry local |
| `internal/api/settings.go` | New runtime fields |
| `internal/api/server.go` | `/api/models`, `/api/quotas`, settings |
| `internal/api/web/*` | Nav rename, Models page, settings form, routes |
| `configs/config.example.yaml` / docker | Example opencode-go provider |

---

### Task 1: Quota types + OpenCode Go fetcher

**Files:**
- Create: `internal/provider/quota.go`
- Create: `internal/provider/quota_opencode_go.go`
- Create: `internal/provider/quota_opencode_go_test.go`
- Modify: `internal/config/config.go` — add `Quota string \`yaml:"quota"\``

**Interfaces:**
- Produces: `type QuotaWindow struct { UsedPercent float64; ResetsAt time.Time; Status string }`
- Produces: `type QuotaSnapshot struct { ProviderID string; Rolling, Weekly, Monthly *QuotaWindow; FetchedAt time.Time; Source string }`
- Produces: `type QuotaFetcher interface { FetchQuota(ctx context.Context) (QuotaSnapshot, error) }`
- Produces: `func NewOpenCodeGoQuota(providerID, baseURL, apiKey string) *OpenCodeGoQuota`

- [ ] **Step 1: Write failing test** for parsing Go usage JSON → UsedPercent

```go
func TestOpenCodeGoQuotaParse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zen/go/v1/usage" && r.URL.Path != "/v1/usage" {
			// accept either full path or base already including /zen/go
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"usage":{"rolling":{"percent":10,"resetsAt":"2026-10-18T00:00:00Z","status":"ok"},"weekly":{"percent":14,"resetsAt":"2026-10-19T00:00:00Z"},"monthly":{"percent":10,"resetsAt":"2026-11-11T00:00:00Z"}}}`))
	}))
	defer srv.Close()
	q := NewOpenCodeGoQuota("opencode-go", srv.URL, "test-key")
	snap, err := q.FetchQuota(context.Background())
	if err != nil { t.Fatal(err) }
	if snap.Weekly == nil || snap.Weekly.UsedPercent != 14 {
		t.Fatalf("weekly=%v", snap.Weekly)
	}
}
```

- [ ] **Step 2: Run test — expect FAIL** (types missing)

Run: `go test ./internal/provider/ -run OpenCodeGoQuota -count=1`

- [ ] **Step 3: Implement `quota.go` + `quota_opencode_go.go`**
  - GET `{base}/usage` or `{base}/zen/go/v1/usage` depending on whether base already ends with `/zen/go/v1`
  - Auth: `Authorization: Bearer {key}`
  - Map `percent` → `UsedPercent`

- [ ] **Step 4: Run test — PASS**

- [ ] **Step 5: Commit** (only if user asked)

```bash
git add internal/provider/quota*.go internal/config/config.go
git commit -m "feat: OpenCode Go quota fetcher"
```

---

### Task 2: Ollama balance fetcher + QuotaStore headroom

**Files:**
- Create: `internal/provider/quota_ollama_balance.go`
- Create: `internal/provider/quota_store.go`
- Create: `internal/provider/quota_store_test.go`

**Interfaces:**
- Consumes: `QuotaFetcher`, `QuotaSnapshot`
- Produces: `type QuotaStore struct`
- Produces: `func (s *QuotaStore) HeadroomOK(providerID string, thr QuotaThresholds) bool`
- Produces: `func (s *QuotaStore) MarkCooldown(providerID string, until time.Time)`
- Produces: `func (s *QuotaStore) InCooldown(providerID string, now time.Time) bool`
- Produces: `type QuotaThresholds struct { Rolling, Weekly, Monthly float64 }`

- [ ] **Step 1: Failing tests**
  - `TestHeadroomOK_AllUnderThreshold`
  - `TestHeadroomOK_FailsWhenWeeklyOver`
  - `TestHeadroomOK_CooldownBlocks`
  - `TestHeadroomOK_NoSnapshotDisablesEscalation` (adapter registered but no snap → false)
  - `TestHeadroomOK_NoAdapterAllows` (provider not in fetcher map → true if no cooldown)

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement store + ollama balance**
  - Ollama: `remaining_percent` → `UsedPercent = 100 - remaining`
  - `Refresh(ctx)` loops fetchers
  - Defaults thr 85/90/95

- [ ] **Step 4: Tests PASS**

- [ ] **Step 5: Commit** (if asked)

---

### Task 3: PerformanceStore + enrich Models

**Files:**
- Create: `internal/provider/perf_store.go`
- Create: `internal/provider/perf_store_test.go`
- Modify: `internal/store/usage.go` — helper `ModelLatencyStats(modelID, since) (p50 float64, n int, err error)` if missing
- Modify: `internal/provider/registry.go` — `Models()` copies + applies PerfStore metrics; parse Params from name on sync

**Interfaces:**
- Produces: `func (p *PerfStore) Observe(modelID string, latencyMS int64, completionTokens int)`
- Produces: `func (p *PerfStore) Apply(m *ModelInfo)` sets `Metrics["latency_p50"]`, `Metrics["tps"]` when known
- Consumes: optional DB reload on interval

- [ ] **Step 1: Test** Observe two latencies → Apply sets latency_p50; tps when tokens+latency known

- [ ] **Step 2: FAIL → implement → PASS**

- [ ] **Step 3: Wire** `Registry.SetPerfStore` / apply in `Models()` and `Get`

- [ ] **Step 4: Commit** (if asked)

---

### Task 4: Settings fields for escalation

**Files:**
- Modify: `internal/api/settings.go`
- Modify: `internal/api/settings_test.go` (create if needed)

**Interfaces:**
- Extends `RuntimeSettings`:
  - `EscalateMinComplexity string` default `"hard"`
  - `QuotaThresholdRollingPct float64` default `85`
  - `QuotaThresholdWeeklyPct float64` default `90`
  - `QuotaThresholdMonthlyPct float64` default `95`
  - `QuotaCooldownSeconds int` default `600`

- [ ] **Step 1: Test defaults after NewSettingsStore empty dir**

- [ ] **Step 2: Implement defaults + Put/Get roundtrip**

- [ ] **Step 3: PASS + commit if asked**

---

### Task 5: Scorer + Decide escalation

**Files:**
- Modify: `internal/router/scorer.go`
- Modify: `internal/router/decide.go`
- Modify: `internal/router/decide_test.go` / create `scorer_test.go`

**Interfaces:**
- Extends `DecideInput`:
  - `EscalateMin string`
  - `ExternalOK func(providerID string) bool` // headroom && !cooldown
- Scorer: if escalate eligible and `CostTier=="metered"` and ExternalOK(provider) → large bonus (e.g. +80); if escalate and local → smaller bonus; complexity×params + Metrics latency (prefer lower latency: `score += max(0, 50 - p50/100)`)

- [ ] **Step 1: Test** hard + external OK → picks metered over local  
- [ ] **Step 2: Test** hard + external NOT OK → local  
- [ ] **Step 3: Test** simple + external OK → still local preferred  
- [ ] **Step 4: Test** sticky local + hard escalate → StickyBreak and external (or document: clear sticky when tier mismatch)

Sticky rule (spec): if sticky model is local but escalate wants external (and ExternalOK), ignore sticky and rescore (`StickyBreak=true`).

- [ ] **Step 5: Implement → PASS**

- [ ] **Step 6: Commit** (if asked)

---

### Task 6: Proxy 429 → cooldown + local retry

**Files:**
- Modify: `internal/proxy/chat.go`
- Modify: `internal/proxy/chat_test.go`
- Modify: `cmd/router/main.go` — wire QuotaStore, PerfStore, settings into Deps

**Interfaces:**
- Extends `Deps`: `Quotas *provider.QuotaStore`, `Perf *provider.PerfStore`, `Settings func() api.RuntimeSettings` or thin callback with escalate fields
- On upstream status 429: `Quotas.MarkCooldown`, re-Decide with ExternalOK always false for that provider / force local-only once, return second response
- On success: `Perf.Observe(model, latency, completionTokens)`

- [ ] **Step 1: httptest upstream returns 429 then local 200 — expect one cooldown mark + final 200**

- [ ] **Step 2: Implement minimal retry path**

- [ ] **Step 3: PASS**

- [ ] **Step 4: Commit** (if asked)

---

### Task 7: API `/api/models` + `/api/quotas`

**Files:**
- Modify: `internal/api/server.go`
- Modify: `internal/api/auth.go` — public? no, same as inventory (auth when non-loopback)
- Create: `internal/api/models_api_test.go`

**Interfaces:**
- `GET /api/models` → `[{id, provider_id, upstream_id, display_name, cost_tier, routable, tools, params, metrics, benchmark_url, ollama_url}]`
- `GET /api/quotas` → `{providers: [...], fetched_at}`
- Sync already exists; call `Quotas.Refresh` inside sync handler

- [ ] **Step 1: Test** Server with fake registry returns models JSON

- [ ] **Step 2: Implement handlers**

- [ ] **Step 3: PASS + commit if asked**

---

### Task 8: UI — rename Рекомендации, new Модели, settings

**Files:**
- Modify: `internal/api/web/index.html`
- Modify: `internal/api/web/app.js`
- Modify: `internal/api/web/styles.css` (minimal)
- Modify: `internal/api/ui.go` — SPA paths already include `/models` and `/recommendations`; ensure mapping:
  - tab `recommendations` → `/recommendations`
  - tab `models` → `/models` (NEW page)
- Modify: `internal/api/auth.go` public paths if needed

**UI steps:**
- [ ] Nav: Usage | Качество | **Модели** | **Рекомендации** | Настройки
- [ ] Move current recommendations section id to `recommendations`; path `/recommendations`
- [ ] New section `models`: table from `GET /api/models`, quota cards from `GET /api/quotas`, refresh button
- [ ] Settings form fields for escalate + three thresholds + cooldown
- [ ] `PATH_TAB` / `TAB_PATH` update; hard-refresh friendly
- [ ] Manual check: `curl` HTML contains both tabs; JS has `loadModelsPage`

- [ ] **Commit** if asked

---

### Task 9: Config example + docker wiring

**Files:**
- Modify: `configs/config.example.yaml`
- Modify: `configs/config.docker.yaml` (commented opencode-go example)
- Modify: `cmd/router/main.go` — register QuotaFetchers from `pc.Quota`, StartPeriodicRefresh 60s

- [ ] Document env `OPENCODE_API_KEY`
- [ ] `make docker-up` / `go test ./...` green

- [ ] **Commit** if asked

---

### Task 10: Verification

- [ ] `go test ./... -count=1`
- [ ] Manual: settings escalate=hard, mock or real Go usage under threshold → hard chat picks external if configured
- [ ] UI `/models` shows quotas; `/recommendations` shows old recommend UI
- [ ] 429 path: cooldown visible in `/api/quotas`

---

## Execution note

После утверждения плана: `subagent-driven-development` или `executing-plans`, task-by-task. Не начинать код до явного «делай» / «implement» от пользователя, если он ещё не дал зелёный свет на реализацию.
