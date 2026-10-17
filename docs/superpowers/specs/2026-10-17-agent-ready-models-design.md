# Agent-Ready Models (OpenCode) Design

Date: 2026-10-17  
Status: approved for implementation

## Goal

Stop routing tool-using agent traffic (OpenCode via `router/auto`) to models that are too weak or unsuitable. Show readiness on Models UI with auto rules plus manual allow/deny override.

## Decisions

| Topic | Choice |
|-------|--------|
| Suitability | Auto rules + manual override (allow/deny/auto) |
| When enforced | Only when request body has `tools` |
| Auto min params | ≥ 14B (setting `agent_min_params`) |
| Auto name deny | `(?i)(vl|vision|abliterated)` unless force_allow |
| Probe tool-call | Not in v1 |

## Effective agent_ok

1. If override `force_deny` → false (`source=deny`)
2. Else if override `force_allow` → true (`source=allow`)
3. Else auto (`source=auto`):
   - `Tools == true`
   - `Params >= agent_min_params` (estimate from name if Params==0; if still 0 → fail closed)
   - name does not match VL/abliterated pattern

## Routing

In `filterModels` / Decide: when `class.NeedsTools` (from tools present), drop models with `!agent_ok`.  
Sticky hit on denied model → sticky break + rescore.  
If no candidates left: try escalate path (EscalateMin effectively open + ExternalOK) once; else HTTP 503 JSON `no_agent_ready_model`.

Chat without tools unchanged.

## Persistence

`{data_dir}/model_overrides.json`:

```json
{
  "ollama/qwen2.5-coder:7b": "deny",
  "ollama/gemma4:31b-mlx": "allow"
}
```

Values: `auto` | `allow` | `deny`. Missing key = auto.

## Settings

`agent_min_params` int, default `14`, in RuntimeSettings + Settings UI.

## API / UI

- `GET /api/models` adds `agent_ok`, `agent_ok_source`, `agent_ok_reason`
- `PUT /api/models/overrides` body `{ "id": "provider/model", "override": "auto"|"allow"|"deny" }`
- Models table: OpenCode OK badge + select

## Tests

- Auto: 7B false, 31B true, VL false
- Override allow on 7B → true; deny on 31B → false
- Decide with NeedsTools skips 7B
- Sticky break on denied pin
