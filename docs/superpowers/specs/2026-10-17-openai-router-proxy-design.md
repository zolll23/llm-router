# OpenAI Router Proxy — Design Spec

**Date:** 2026-10-17  
**Status:** Draft for user review  
**Related:** research `~/Documents/Go_OpenAI_Router_Proxy_Research_20261017/`; implementation plan skeleton `docs/superpowers/plans/2026-10-17-openai-router-proxy.md` (to be rewritten after this spec is approved)

---

## 1. Goal

Build a single Go binary that acts as an OpenAI-compatible proxy. Clients (especially OpenCode) select a virtual model `auto`. The proxy classifies each request, chooses a concrete backend from a multi-provider inventory, prefers local/cheap capacity for simple work, tracks usage and routing quality, and exposes a small web UI.

Success for v1 ship:

- OpenCode can use `router/auto` with tools + SSE streaming.
- Providers are configured as a list; models are discovered automatically and namespaced so collisions never merge in stats or balancing.
- Usage overview (day/week/month) works per `provider/model`.
- Quality tab shows online routing signals; AI-judge + GLiNER fine-tune export land in phase 2 of the same product.

---

## 2. Scope

### In scope (v1 core)

- `GET /v1/models`, `POST /v1/chat/completions` (SSE), `GET /health`
- Provider adapters + background inventory sync
- Nimble classifier (Ollama `/v1/systemone`) + heuristic fallback
- Local-first scoring, sticky sessions, Ollama `keep_alive` for pinned local models
- SQLite usage store + Usage UI
- Settings UI (classifier, thresholds, sticky TTL, listen/token, provider status)
- Quality UI with online signals (escalations, overrides, local share)
- Auth model: loopback open; non-loopback requires token

### In scope (phase 2, same spec)

- Periodic AI-judge job (Nimble/clef) → `judge_labels`
- Confusion matrix / calibration APIs and UI
- GLiNER JSONL export (human gate before train)
- Cursor chat routing if a stable inference path is found (otherwise discover-only)
- Optional parse expansion / child upstreams from OpenCode config (adapter already listed)

### Out of scope

- `/v1/completions`, `/v1/embeddings`, `/v1/responses` until a real client needs them
- Cross-model KV cache transfer / learned mappers
- Semantic response cache for agent traffic
- Automatic GLiNER retrain+deploy without human review
- Multi-tenant SaaS, Redis cluster, OIDC login
- Guaranteed Cursor inference (best-effort only)

---

## 3. Architecture

```
Clients (OpenCode, etc.)
    │  /v1/models, /v1/chat/completions
    ▼
API (+ optional Bearer token)
    │  virtual model: auto
    ▼
Pipeline: Inventory → Classify → Filter → Score → Sticky → Proxy
    │                                    │
    ▼                                    ▼
Provider adapters                 SQLite: usage_events,
(Ollama, OpenAI-compatible,       routing_events, judge_labels
 OpenCode config, Cursor CLI)            │
    │                                    ▼
    ▼                             Web UI + /api/*
Upstream Chat (if routable)
```

Principles:

- Hot path never waits on judge/export jobs.
- Canonical backend ID is always `provider_id/upstream_model_id`.
- One binary, embedded UI, SQLite on local disk.

---

## 4. Provider inventory

### Configuration

Providers are declared in config (YAML). Models are not hand-listed for discovery-capable types.

```yaml
listen: "127.0.0.1:8080"
admin_token: ""          # required if listen is not loopback
api_token: ""            # optional; defaults to admin_token for /v1/*
virtual_model: auto
data_dir: "~/.local/share/router"

providers:
  - id: ollama
    type: ollama
    base_url: "http://127.0.0.1:11434"
  - id: openai
    type: openai_compatible
    base_url: "https://api.openai.com/v1"
    api_key_env: OPENAI_API_KEY
  - id: opencode
    type: opencode_config
    config_path: "~/.config/opencode/opencode.json"
  - id: cursor
    type: cursor_cli
    command: "agent"
```

### Provider interface

| Method | Required | Behavior |
|--------|----------|----------|
| `ListModels(ctx)` | yes | Returns catalog entries |
| `ChatCompletions(ctx, req)` | no | If missing → `routable=false` |
| `Health(ctx)` | recommended | Feeds scorer |

### ModelInfo fields

- `ProviderID`, `UpstreamID`, `DisplayName`
- `Routable` bool
- Capabilities: tools, vision, context length (best-effort)
- `CostTier`: `local` | `metered` | `unknown`

### Canonical ID

`{provider_id}/{upstream_id}`  
Examples: `ollama/qwen3-coder`, `openai/gpt-5.2`, `cursor/composer-2.5`.

Stats, sticky pins, and scoring keys use this ID only. UI may show short name + provider badge.

### Adapter notes

| Type | ListModels | Chat |
|------|------------|------|
| `ollama` | `/api/tags` (local + `:cloud` as reported) | OpenAI-compat `/v1/chat/completions` |
| `openai_compatible` | `GET /v1/models` | same base URL |
| `opencode_config` | Parse config; expose declared models and/or nested provider baseURLs as inventory | Proxy to nested upstream when known |
| `cursor_cli` | `agent --list-models` / `agent models` | Phase 2: enable only if stable API path exists; else `routable=false` |

Sync: periodic (default 60s) + manual “Sync now”. Failure of one provider does not block others.

---

## 5. Request pipeline

### Virtual model

Clients call with `model: auto` (configurable). `GET /v1/models` advertises this virtual model with tools capability and a large context hint (≥64k for OpenCode).

### Overrides

- Header `X-Router-Tier` or body field `router.tier` skips classifier.
- Header `X-Router-Session` supplies sticky key; else hash of stable system-prefix fingerprint.

### Classify

Interface `Classifier`:

1. **Nimble** (default): Ollama `POST /v1/systemone` with `complexity` (trivial/simple/medium/hard) and `needs_tools` (noul). State includes last user text plus signals: `tools_present`, `approx_tokens`, `message_count`.
2. **Heuristic** fallback on timeout/error.
3. **GLiNER** (later): HTTP sidecar; selectable in Settings.

### Filter

Keep backends where `routable=true`, healthy, and capabilities satisfy tools/context needs.

### Score (local-first)

Prefer `CostTier=local` when complexity ∈ {trivial, simple} and a capable local backend exists. Otherwise score by capability fit, remaining quota/headroom (if configured), inverse price, latency EMA, error rate.

### Sticky

Pin chosen canonical ID for TTL (default 30 minutes). Do not reclassify every turn. Break pin on TTL expiry, repeated upstream failure, explicit override, or escalation policy. For Ollama, set long `keep_alive` on pinned model to preserve in-process KV reuse.

### Proxy

- Forward streaming SSE transparently.
- Inject `stream_options.include_usage=true` when streaming.
- Rewrite request `model` to upstream id expected by that provider.
- Response headers: `X-Router-Model`, `X-Router-Provider`, `X-Router-Tier`, `X-Router-Reason`.

### Errors

Upstream failure → try next scored candidate (temporary blacklist). Classifier failure → heuristic + safer (more capable) tier bias.

---

## 6. Data model (SQLite)

Path: `{data_dir}/router.db`.

### usage_events

Per completed request: timestamp, canonical model id, provider id, prompt/cached/completion tokens, latency_ms, session_id, pred complexity/confidence, http status, estimated flag.

### routing_events

Classification + online outcomes: pred fields, chosen model, flags `escalated`, `sticky_break`, `user_override`, `local_success`, truncated state text for later judging.

### judge_labels (phase 2)

`request_id`, gold complexity, underkill/overkill, judge confidence, judge model, judged_at.

Aggregations for Usage UI are computed on read for day/week/month windows (timezone configurable; default local).

---

## 7. Quality loop

### Phase 1 (ship with Quality tab)

Online-only metrics:

- Local request share %
- Escalation rate
- User override rate
- Sticky break rate
- Paid tokens on predicted-simple (overkill proxy)

### Phase 2

Background scheduler samples events (priority: escalated → low confidence → paid-on-simple → random). Judge via Nimble/clef `systemone`. Persist labels. Expose:

- `GET /api/eval/summary`
- `GET /api/eval/confusion`
- `GET /api/eval/calibration`
- Export GLiNER JSONL (`complexity` head) with confidence filter; human reviews before training.

Judge never blocks chat.

---

## 8. Web UI and security

### Tabs

1. **Usage** — rows = canonical models; columns day/week/month; cells = in/cache/out/reqs/share%; click → detail charts (load + latency percentiles).
2. **Качество** — online KPIs; phase 2 adds matrix and examples.
3. **Настройки** — classifier, thresholds, sticky TTL, listen address, tokens, provider list/status, sync.

UI language: Russian. Code and config keys: English.

### Security

- Default listen `127.0.0.1`: no auth.
- Non-loopback listen: require `admin_token` for UI and `/api/*`; `/v1/*` requires `api_token` (or `admin_token` if api unset).
- Provider secrets only via environment variables referenced in config.

---

## 9. API surface (normative)

| Method | Path | Auth | Notes |
|--------|------|------|-------|
| GET | `/health` | none | liveness |
| GET | `/v1/models` | api if non-loopback | includes `auto` |
| POST | `/v1/chat/completions` | api if non-loopback | main path |
| GET | `/api/inventory` | admin | discovered backends |
| GET/PUT | `/api/settings` | admin | runtime settings |
| GET | `/api/stats/overview` | admin | usage table |
| GET | `/api/stats/models/{id}` | admin | detail |
| GET | `/api/eval/*` | admin | phase 2 (+ phase 1 stubs OK) |
| GET | `/` | admin if non-loopback | UI |

---

## 10. Decisions log

| Topic | Choice |
|-------|--------|
| v1 scope shape | Core + Quality online signals; AI-judge/export phase 2 |
| Admin exposure | Loopback open; token required off-loopback |
| Inventory | Provider list → auto discovery; namespaced IDs |
| Architecture | Provider adapters in-process (not LiteLLM wrap) |
| Cursor | Discover via CLI; route only if stable chat path exists |
| OpenAI extra endpoints | Chat + models only for v1 |
| Classifier default | Nimble via Ollama; GLiNER later |

---

## 11. Testing strategy

- Unit: scorer local-first rules; canonical ID formatting; sticky TTL.
- HTTP tests with fake providers for ListModels + Chat.
- Nimble classifier against httptest mock of `/v1/systemone`.
- Manual: OpenCode config pointing at `http://127.0.0.1:8080/v1`, model `auto`, tools exercise.
- Eval: fixture routing_events → judge mock → confusion counts deterministic.

---

## 12. Rollout phases

| Phase | Deliverable |
|-------|-------------|
| P0 | Health + chat proxy to one Ollama model + `/v1/models` |
| P1 | Multi-provider inventory + namespaced IDs |
| P2 | Nimble + local-first + sticky |
| P3 | Usage SQLite + Usage UI |
| P4 | Settings + Quality online signals |
| P5 | AI-judge + confusion + GLiNER export |
| P6 | Cursor chat probe; quotas polish |

---

## 13. Resolved defaults / remaining spike

1. **OpenCode nested providers:** flatten to `opencode_<name>/<model>` so stats and balancing never merge across upstreams.
2. **Day boundaries:** local OS timezone; optional override in Settings.
3. **Cursor inference transport:** research spike in P6 only; until a stable path exists, Cursor models stay `routable=false` (discoverable in inventory/UI).
