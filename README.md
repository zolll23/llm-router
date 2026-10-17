# OpenAI Router Proxy

Go OpenAI-compatible proxy that routes requests across providers (Ollama, OpenAI-compatible, OpenCode config, Cursor CLI inventory) using a classifier and local-first policy.

See:

- Design: [`docs/superpowers/specs/2026-10-17-openai-router-proxy-design.md`](docs/superpowers/specs/2026-10-17-openai-router-proxy-design.md)
- Plan: [`docs/superpowers/plans/2026-10-17-openai-router-proxy.md`](docs/superpowers/plans/2026-10-17-openai-router-proxy.md)

## Quick start

### Docker (router + GLiNER2.5)

```bash
make docker-up
# UI: http://127.0.0.1:8080/  (UI sends Bearer local automatically)
# GLiNER: http://127.0.0.1:8090/health
curl -s -H 'Authorization: Bearer local' http://127.0.0.1:8080/v1/models
```

Default classifier is **GLiNER2.5** (`services/gliner`). First start downloads the model into a Docker volume (can take several minutes).

### Local Go only

```bash
go run ./cmd/router -config configs/config.example.yaml
# UI: http://127.0.0.1:8080/
curl -s http://127.0.0.1:8080/health
curl -s http://127.0.0.1:8080/v1/models
```

Requires Ollama with at least one local model for chat. With Docker, point GLiNER base URL at `http://gliner:8090` (compose sets this via env). Fallback: heuristic.

## OpenCode

```json
{
  "$schema": "https://opencode.ai/config.json",
  "model": "router/auto",
  "provider": {
    "router": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "Local Router",
      "options": {
        "baseURL": "http://127.0.0.1:8080/v1",
        "apiKey": "local"
      },
      "models": {
        "auto": {
          "name": "Smart Router",
          "limit": { "context": 128000, "output": 16384 }
        }
      }
    }
  }
}
```

## Features in tree

- Multi-provider inventory (namespaced `provider/model`)
- GLiNER2.5 classify (Docker sidecar) + nimble/heuristic options + local-first score + sticky sessions
- Usage SQLite + web UI (Usage / Качество / Настройки)
- `POST /api/eval/export-finetune` → GLiNER JSONL under `data_dir/finetune/`
