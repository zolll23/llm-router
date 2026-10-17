# Local Memory Queue Design

Date: 2026-10-17  
Status: approved for implementation

## Goal

When concurrent local Ollama requests need different models and the host cannot hold them all in memory, the router must **queue** requests (holding the client HTTP connection) instead of blindly forwarding and risking OOM / thrashing. Overflow beyond a configured queue depth either redirects to external providers (quota headroom permitting) or returns 429.

## Non-goals (v1)

- Per-model isolated queues as separate memory pools
- Changing Ollama’s own `OLLAMA_MAX_QUEUE` / `NUM_PARALLEL` configuration
- Hard wait timeout once a request is admitted to the wait queue (wait until client cancel or success)
- UI visualization of the live queue (optional later)
- Explicit `local_mem_budget_gb` override in settings (use host hardware profile; override later if needed)

## Decisions (locked)

| Topic | Choice |
|-------|--------|
| Client while waiting | Hold HTTP connection open |
| Memory signal | Hybrid: router reservation + periodic/on-release sync with Ollama `GET /api/ps` |
| Wait once queued | Until completion or client disconnect |
| Queue shape | **One** wait queue per local Ollama provider (shared memory budget) |
| Wake policy | Prefer waiters whose model is already loaded; else FIFO |
| Overflow | If `len(waiting) ≥ local_queue_max`: if `local_queue_redirect` and external headroom OK → metered; else **429** |
| External on overflow | Same headroom gates as escalation; no headroom → 429 (do not re-enter local queue in the same request) |

## Architecture

New in-process component: `LocalScheduler` (one instance per local `type: ollama` provider, or one shared scheduler keyed by provider id + base URL).

### Request path

1. Classify + `Decide` as today (sticky, tools filter, escalate scoring, etc.).
2. If chosen model is **not** local Ollama → proxy immediately (scheduler unused).
3. If local Ollama:
   - Estimate `needGB` from catalog (`VRAMReq` / params heuristic, same family as recommendations).
   - Snapshot: reserved in-flight + `/api/ps` sizes for loaded models.
   - **Fits** (same model already loaded with free parallel slot, or free budget ≥ need after accounting for other loaded models) → `Acquire` → proxy → `Release` when response finishes (including stream end / client abort).
   - **Does not fit** → try admit to wait queue:
     - if waiting length ≥ `local_queue_max` → overflow path (redirect or 429);
     - else enqueue, hold connection, block until acquired or context cancelled.
4. On every `Release` and on a short poll tick: wake eligible waiters (prefer already-loaded model).

### Memory budget

- Source: host hardware detection (existing `host_hardware.json` / Darwin detect).
- Unified memory (Apple Silicon): `budgetGB = floor(RAM_GB * 0.72)` (aligned with recommendations).
- Discrete: use detected VRAM GB.
- Fallback if unknown: conservative default (e.g. 12 GB) + log.

### Fit rules (v1)

- **Same upstream model already in `/api/ps` (or reserved as loaded):** admit at most **1** in-flight request per model (hardcoded). No extra KV padding in v1. Parallelism beyond that stays Ollama’s concern only after we choose to raise the cap later.
- **New model:** admit only if `usedGB + needGB ≤ budgetGB`, where `usedGB` is max(sum of sizes from `/api/ps`, sum of our in-flight reservations for other models). Do **not** assume Ollama will unload mid-request; wait until another request’s `Release` and/or ps shows unload.
- If `/api/ps` fails: rely on router reservations + catalog sizes; log warning; always `Release` on completion so reservations cannot leak.

### Overflow

```
if cannot Acquire immediately && waiting >= local_queue_max:
  if local_queue_redirect && ExternalOK(provider):
    re-Decide / force local_only=false path → best metered → proxy
    reason = queue_overflow_redirect
  else:
    HTTP 429
    reason = queue_overflow
```

No second local queue attempt in the same request after overflow redirect fails.

## Settings

Extend `RuntimeSettings` + Settings UI:

| Field | Type | Default | Meaning |
|-------|------|---------|---------|
| `local_queue_max` | int | `8` | Max **waiting** (not in-flight) entries |
| `local_queue_redirect` | bool | `false` | Overflow → try external with headroom, else 429 |

Persist in existing `settings.json`. Wire into `proxy.RoutingSettings` / `Deps`.

## Observability

Response headers (when applicable):

- `X-Router-Queue-Wait-Ms` — time spent waiting before upstream start
- `X-Router-Reason` — includes `queued`, `queue_overflow_redirect`, `queue_overflow` as appropriate

Usage / sankey: overflow 429 uses existing limit-style status labeling; redirect records chosen external model normally.

Optional later: `GET /api/queue` snapshot for UI.

## Components (implementation sketch)

| Piece | Role |
|-------|------|
| `internal/provider/ollama_ps.go` (or under scheduler) | Fetch `/api/ps`, parse model name + size bytes |
| `internal/scheduler/local.go` | Acquire / Release / enqueue / wake; budget + fit |
| `internal/proxy/chat.go` | Gate local paths through scheduler; overflow branch |
| `internal/api/settings.go` + web Settings | New fields |
| Tests | Fit math; queue depth → 429; redirect with mocked ExternalOK; cancel removes waiter |

## Edge cases

| Case | Behavior |
|------|----------|
| Client disconnect while waiting | Remove from queue; wake others |
| Sticky pin to heavy model while light model loaded | Still queue until fit; sticky does not bypass memory gate |
| Only metered providers configured | Scheduler unused |
| Queue max = 0 | No waiting: immediate overflow path (redirect or 429) |
| Two Ollama providers (unlikely) | Separate scheduler per provider id |

## Testing

- Unit: budget/fit with fake ps snapshots; FIFO + prefer-loaded wake order; max depth.
- Proxy integration with fake upstream: hold then proceed; overflow 429; overflow redirect when headroom stub true.
- No requirement to run real Ollama in CI.

## Rollout

Feature always on for local ollama once shipped; disable effectively with very high `local_queue_max` and accepting waits, or by not using local providers. No separate feature flag required in v1.
