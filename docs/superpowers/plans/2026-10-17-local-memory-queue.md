# Local Memory Queue Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Queue local Ollama chat requests when models would not fit in host memory; hold HTTP; overflow → external (headroom) or 429.

**Architecture:** In-process `LocalScheduler` per Ollama provider: hybrid `/api/ps` + reservations, one wait queue, prefer already-loaded wake. Proxy gates local forwards; settings expose `local_queue_max` and `local_queue_redirect`.

**Tech Stack:** Go, existing proxy/router/settings/UI, Ollama `GET /api/ps`.

## Global Constraints

- One wait queue per Ollama provider (shared budget)
- Hold HTTP while waiting; no wait timeout once admitted
- Max 1 in-flight per model (v1)
- Overflow: redirect with headroom else 429
- Budget: unified RAM×0.72 else VRAM; default 12 if unknown

---

### Task 1: Ollama `/api/ps` + LocalScheduler + tests

**Files:**
- Create: `internal/provider/ollama_ps.go`
- Create: `internal/scheduler/local.go`
- Create: `internal/scheduler/local_test.go`

**Interfaces:**
- Produces: `provider.FetchOllamaPS(ctx, baseURL) (map[string]float64, error)` sizes in GB
- Produces: `scheduler.NewLocal(budgetGB float64, fetchPS func(context.Context) (map[string]float64, error)) *Local`
- Produces: `(*Local).Acquire(ctx, upstreamID string, needGB float64, queueMax int) (waitMS int64, release func(), err error)` with `ErrOverflow`
- Produces: `scheduler.EstimateNeedGB(upstreamID string, params int) float64`

- [ ] Implement + unit tests for fit, overflow, prefer-loaded wake, cancel
- [ ] Commit

### Task 2: Settings + UI fields

**Files:**
- Modify: `internal/api/settings.go`
- Modify: `internal/api/web/index.html`, `app.js`

- [ ] Add `local_queue_max` (default 8), `local_queue_redirect` (default false)
- [ ] Commit

### Task 3: Wire proxy + main + server

**Files:**
- Modify: `internal/proxy/chat.go`, `CopyAndRecord`
- Modify: `internal/api/server.go`
- Modify: `cmd/router/main.go`
- Create: `internal/proxy/queue_test.go` (or extend chat_test)

- [ ] Gate local ollama through scheduler; Release after copy; overflow path
- [ ] Headers `X-Router-Queue-Wait-Ms`, reason codes
- [ ] `go test ./...`
- [ ] Commit
