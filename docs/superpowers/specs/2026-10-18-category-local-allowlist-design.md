# Category local allowlist

## Goal

For each classifier category (`coding|reasoning|chat|creative`), optionally restrict **local** models to an allowlist. Cloud models stay unrestricted (subject to existing escalate/quota/queue-overflow redirect).

## Rules

1. **Per-category allowlist** of local model IDs (`provider/upstream`, e.g. `ollama/gpt-oss:20b`).
2. **Empty / missing list for category** → current behavior (scorer + `agent_ok` / repair gate).
3. **Non-empty list** → local candidates must be in the list. Cloud candidates unchanged.
4. **Loaded gate**: if the chosen local model is **not** present in Ollama `/api/ps`, do **not** enqueue a cold load — fall back to cloud (or overflow path). If it **is** loaded but busy, use existing `Acquire` + `local_queue_max` / `local_queue_redirect`.
5. **Sticky**: valid only if sticky model is still allowed (in allowlist when list non-empty). For local sticky, also require loaded (else sticky break + re-decide).

## Settings

```json
"local_allowlist": {
  "coding": ["ollama/gpt-oss:20b", "ollama/gemma4:31b-mlx"],
  "chat": ["ollama/gemma4:12b-mlx"]
}
```

UI: Settings checkboxes of local inventory models per category.

## Flow

Classify → Decide (filter local by allowlist) → sticky checks → if local && !loaded → Decide again excluding locals / cloud-only → else Acquire as today.

## Out of scope

Per-model priority weights inside allowlist; cloud allowlists; changing classifier labels.
