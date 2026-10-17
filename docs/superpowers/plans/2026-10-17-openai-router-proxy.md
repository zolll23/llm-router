# OpenAI Router Proxy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Go OpenAI-compatible proxy that OpenCode can select as model `router/auto`, classifies requests (Nimble via Ollama, later GLiNER), routes local-first with quota/limits + sticky sessions, tracks usage, and exposes a small web UI.

**Architecture:** Single Go binary: HTTP API (`/v1/*` + `/api/*` + embedded UI) → Inventory → Classifier → Scorer → StickyStore → UpstreamProxy (SSE) → SQLite usage + routing eval store. Periodic EvalWorker (AI-judge via Nimble/clef or cheap LLM) builds confusion matrices, underkill/overkill stats, and fine-tune JSONL candidates. Backends: Ollama local/cloud and OpenAI-compatible URLs from config / `opencode.json`.

**Tech Stack:** Go 1.22+, `net/http` or chi, modernc.org/sqlite, embed.FS for UI, Ollama `/v1/systemone` for Nimble (classifier + cheap judge).

**Research basis:** `~/Documents/Go_OpenAI_Router_Proxy_Research_20261017/`

## Global Constraints

- OpenAI-compatible surface for OpenCode: primarily `POST /v1/chat/completions` + `GET /v1/models` + SSE streaming.
- Do **not** implement `/v1/completions`, `/v1/embeddings`, `/v1/responses` in MVP (see Endpoint Policy below).
- Classifier default: Nimble via Ollama `POST /v1/systemone` (Ollama ≥ 0.35).
- Local-first for simple requests; sticky session to preserve Ollama KV reuse.
- Always inject `stream_options.include_usage` on upstream streamed requests.
- Russian UI labels OK; code/comments in English.
- Single-user desktop first (SQLite, no Redis required).

## Endpoint Policy

| Endpoint | MVP | Later | Why |
|----------|-----|-------|-----|
| `GET /v1/models` | **Yes** | — | OpenCode picker / some providers require IDs to match |
| `POST /v1/chat/completions` | **Yes** | — | OpenCode `@ai-sdk/openai-compatible` path |
| `GET /health` | **Yes** | — | Ops |
| `GET/PUT /api/settings` | **Yes** | — | Classifier choice |
| `GET /api/stats/*` | **Yes** | — | Usage UI |
| `GET /api/inventory` | **Yes** | — | Debug backends |
| `POST /v1/completions` | No | Only if a client needs legacy Completions | OpenCode does not use it |
| `POST /v1/embeddings` | No | Only for semantic cache / RAG | Not needed for agent routing |
| `POST /v1/responses` | No | If OpenCode points `@ai-sdk/openai` Responses models through this proxy | Different package/path; out of MVP |

**Rule:** Implement new `/v1/*` only when a real client in the inventory requires it. Prefer chat completions + transparent proxy headers.

---

## File Structure

```
cmd/router/main.go
internal/config/config.go
internal/api/server.go          # route mount
internal/api/openai_models.go
internal/api/openai_chat.go     # chat + SSE
internal/api/admin_settings.go
internal/api/admin_stats.go
internal/inventory/ollama.go
internal/inventory/opencode.go
internal/inventory/merge.go
internal/classifier/classifier.go
internal/classifier/nimble.go
internal/classifier/heuristic.go  # fallback
internal/router/scorer.go
internal/router/sticky.go
internal/router/decide.go
internal/proxy/upstream.go        # reverse proxy + usage harvest
internal/usage/store.go           # SQLite
internal/usage/aggregate.go
internal/eval/store.go            # routing_events, judge_labels, matrices
internal/eval/signals.go          # online outcome signals
internal/eval/judge.go            # Nimble/LLM-as-judge
internal/eval/scheduler.go        # periodic analysis jobs
internal/eval/export_finetune.go  # GLiNER JSONL export
web/                              # embed UI
  index.html
  app.js
  styles.css
configs/config.example.yaml
go.mod
README.md
```

---

## Phase 0 — Skeleton

### Task 0: Module + hello server

- [ ] `go mod init` (module path e.g. `github.com/pahomov/router` or local module name)
- [ ] `cmd/router/main.go` listens `:8080`, `GET /health` → `{"ok":true}`
- [ ] Example `configs/config.example.yaml` with `listen`, `ollama_base`, `virtual_model: auto`
- [ ] Manual: `curl localhost:8080/health`
- [ ] Commit: `chore: bootstrap router module and health endpoint`

---

## Phase 1 — OpenAI chat proxy (unblocks OpenCode)

### Task 1: `GET /v1/models`

- [ ] Return virtual model `auto` (and optional passthrough of discovered Ollama models later)
- [ ] Shape: OpenAI `{object:"list", data:[{id, object:"model", owned_by:"router"}]}`
- [ ] Test: table test on JSON shape
- [ ] Commit: `feat: expose virtual model via /v1/models`

### Task 2: Naive proxy `POST /v1/chat/completions`

- [ ] Parse body; if `model == auto` (or configured virtual id), rewrite to default upstream Ollama model from config
- [ ] Forward to `{ollama}/v1/chat/completions` with streaming passthrough
- [ ] Non-stream: buffer + return JSON
- [ ] Inject `stream_options.include_usage=true` when `stream=true`
- [ ] Manual: OpenCode provider pointed at `http://127.0.0.1:8080/v1` with model `auto`
- [ ] Commit: `feat: proxy chat completions to Ollama`

### Task 3: OpenCode config snippet in README

- [ ] Document v1-style `provider.router` + `baseURL` + model `auto` with `tools: true`, context ≥ 64k
- [ ] Commit: `docs: OpenCode integration snippet`

---

## Phase 2 — Classification + local-first routing

### Task 4: Inventory from Ollama

- [ ] Poll `GET {ollama}/api/tags` on interval; mark `:cloud` vs local
- [ ] Store `Backend{ID, Source, Caps, Healthy}`
- [ ] `GET /api/inventory` for debug
- [ ] Commit: `feat: ollama model inventory`

### Task 5: Nimble classifier

- [ ] `NimbleClassifier` → `POST {ollama}/v1/systemone` with questions: complexity (score), needs_tools (noul)
- [ ] Map answers → `ClassResult{Complexity, NeedsTools, Confidence}`
- [ ] Heuristic fallback if Nimble down / low confidence
- [ ] Unit test with httptest mock
- [ ] Commit: `feat: nimble classifier via systemone`

### Task 6: Scorer + decide

- [ ] Filter backends by tools/context constraints
- [ ] Prefer `local` when complexity ≤ simple
- [ ] Else cloud_included / paid per config tags
- [ ] Response headers: `X-Router-Model`, `X-Router-Tier`, `X-Router-Reason`
- [ ] Commit: `feat: local-first scoring and routing decision`

### Task 7: Sticky sessions

- [ ] Derive `session_id` from hash(system prompt prefix + optional client header `X-Router-Session`)
- [ ] Pin backend for TTL (default 30m); skip full reclassify unless TTL/error/hard task change
- [ ] Set `keep_alive` on Ollama upstream for pinned model
- [ ] Commit: `feat: sticky routing for KV affinity`

---

## Phase 3 — Usage + Web UI

### Task 8: SQLite usage store

- [ ] Schema `usage_events` (ts, model, prompt/cached/completion tokens, latency_ms, session_id, classifier_json)
- [ ] Harvest usage from non-stream JSON and final SSE usage chunk
- [ ] Missing cached_tokens → 0
- [ ] Commit: `feat: sqlite usage accounting`

### Task 9: Stats API

- [ ] `GET /api/stats/overview` → per model × day/week/month × {prompt, cached, completion, reqs, share%}
- [ ] `GET /api/stats/models/{id}` → timeseries + latency p50/p75/p90/p95
- [ ] Commit: `feat: stats aggregation API`

### Task 10: Web UI

- [ ] Embed `web/`: overview table, model detail charts, Settings tab (classifier model, thresholds)
- [ ] `GET /` serves UI; `GET/PUT /api/settings`
- [ ] Commit: `feat: embedded usage and settings UI`

---

## Phase 4 — Hardening

### Task 11: Parse `opencode.json` backends (optional inventory)

- [ ] Read `~/.config/opencode/opencode.json` (+ project config if present)
- [ ] Merge OpenAI-compatible providers as tagged backends
- [ ] Commit: `feat: inventury from opencode config`

### Task 12: Limits / concurrency awareness

- [ ] Config: per-backend rpm/concurrent + optional Ollama cloud credit headroom (manual or poll usage API later)
- [ ] Scorer penalties when saturated; fail-open to local
- [ ] Commit: `feat: quota-aware scoring`

### Task 13: GLiNER sidecar adapter (optional)

- [ ] `Classifier` interface already pluggable; HTTP client to sidecar `/classify`
- [ ] Settings UI switch nimble|gliner|heuristic
- [ ] Commit: `feat: gliner classifier backend`

### Task 14: Exact response cache (opt-in, off for tools)

- [ ] Hash full request; skip if `tools` present
- [ ] Header `X-Router-Cache: exact`
- [ ] Commit: `feat: optional exact response cache`

---

## Phase 5 — Automated routing quality eval (feedback loop)

Goal: continuously collect classification outcomes, periodically label with a cheap AI-judge, show confusion matrix / underkill-overkill, and export fine-tune data for GLiNER (or Nimble criteria tuning).

### Online signals (every routed request — free)

On each `/v1/chat/completions` after classify+proxy, append `routing_events`:

```
id, ts, session_id, request_id
state_text          -- truncated user(+signals), redacted if needed
pred_complexity     -- simple|medium|hard (or label set)
pred_needs_tools
pred_confidence
chosen_model, cost_tier
prompt_tokens, completion_tokens, cached_tokens, latency_ms
http_status, tool_calls_in_req, tool_error   -- if detectable
outcome_flags JSON  -- filled async: escalated, sticky_break, user_override
```

Same-session updates (async, no blocking response path):

- **escalated**: within TTL, next turn routed to strictly stronger tier after tool_fail / 5xx / empty tool args
- **sticky_break**: session pin changed for non-TTL reason
- **user_override**: request carried `X-Router-Tier` / forced model
- **local_success**: pred=simple|medium, local backend, no escalate for N following turns

### Periodic AI-judge job

- [ ] Config: `eval.schedule` cron-like (default daily) or interval (`6h`), `eval.sample_rate`, `eval.max_per_run`, `eval.judge_model` (`nimble`|`clef-flash`|chat model)
- [ ] Sample priority queue (not uniform): escalated > low-confidence > random paid-on-simple > random local-on-hard
- [ ] Judge via `/v1/systemone` (preferred, cheap typed):

```
state: { user_text, tools_present, approx_tokens, chosen_tier, outcome_flags }
questions:
  gold_complexity: choice [trivial, simple, medium, hard]
  adequate_model: noul "Was chosen tier sufficient without being wasteful?"
  underkill: noul
  overkill: noul
```

- [ ] Persist `judge_labels(request_id, gold_*, judge_conf, judge_model, judged_at)`
- [ ] Optional second pass: only disagreements (pred ≠ gold) → stronger judge / human review queue file
- [ ] Never block chat path; job runs in background goroutine / `router eval run`

### Aggregates & confusion matrix

- [ ] `GET /api/eval/summary?from=&to=` → counts pred vs gold, underkill%, overkill%, ECE buckets, local_share
- [ ] `GET /api/eval/confusion?label=complexity` → matrix `pred × gold` (+ row/col normalized)
- [ ] `GET /api/eval/calibration` → confidence histogram vs accuracy
- [ ] UI tab **«Качество»**: matrix heatmap, underkill/overkill trend, top disagreement examples
- [ ] Commit: `feat: routing eval store, judge job, confusion API`

### Fine-tune export (GLiNER loop)

- [ ] `POST /api/eval/export-finetune` or CLI `router eval export --format gliner2`
- [ ] Emit JSONL rows:

```json
{"input":"...","output":{"classifications":[{"task":"complexity","labels":["trivial","simple","medium","hard"],"true_label":["simple"]}]}}
```

- [ ] Filters: only `judge_conf ≥ τ` or human-approved; exclude PII-flagged; dedupe near-identical inputs
- [ ] Write to `data/finetune/YYYYMMDD.jsonl` + report of class balance
- [ ] Document manual step: `gliner2` train → reload sidecar; router settings point to new adapter path
- [ ] Commit: `feat: export gliner fine-tune jsonl from judged events`

### Task checklist

- [ ] Schema migration: `routing_events`, `judge_labels`
- [ ] Wire online signal writers into chat handler + session tracker
- [ ] Implement `EvalScheduler` + Nimble judge
- [ ] Confusion / summary APIs + UI tab
- [ ] Fine-tune export
- [ ] Config knobs + README “quality loop”
- [ ] Test: fixture events → judge mock → matrix counts deterministic

---

## Out of scope (explicit)

- Cross-model KV transfer / learned mappers
- Semantic response cache for agent traffic
- `/v1/completions`, `/v1/embeddings`, `/v1/responses` until a client requires them
- Multi-tenant auth / Redis cluster
- Fully automatic GLiNER retrain+deploy without human gate (export + review only)

---

## Suggested milestone order (calendar)

| Milestone | Deliverable | ~Time |
|-----------|-------------|-------|
| M1 | Chat proxy + OpenCode works with fixed upstream | 1–2 d |
| M2 | Nimble + local-first + sticky | 2–3 d |
| M3 | Usage SQLite + overview UI | 2 d |
| M4 | Settings + detail charts + opencode inventory | 2–3 d |
| M5 | Quotas + GLiNER + exact cache | as needed |
| M6 | Eval loop: signals + AI-judge + confusion matrix + finetune export | 2–3 d |

---

## Verification (per milestone)

- M1: OpenCode chat + tools against `router/auto`
- M2: simple prompt → local model header; hard prompt → larger/cloud; sticky keeps model
- M3: after N requests, overview numbers match harvested usage
- M4: settings change classifier without restart
- M5: tools requests never hit exact cache
- M6: after synthetic underkill (force weak model then escalate), event flagged; judge job fills gold; confusion API non-empty; export JSONL valid for gliner2
