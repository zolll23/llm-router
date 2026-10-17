# GLiNER2.5 Local Classifier + Docker Compose Design

Date: 2026-10-17  
Status: approved for implementation

## Goal

Make **GLiNER2.5** the default request classifier, running as a **local Docker service** beside the router in `docker-compose`.

## Scope

In scope:

- `services/gliner/` — FastAPI sidecar wrapping `gliner2[local]` + `fastino/GLiNER2.5-Decide`
- Root `docker-compose.yml` with `gliner` + `router`
- Go client `internal/classifier/gliner.go` with heuristic fallback
- Settings default `classifier=gliner`; UI option labeled GLiNER2.5
- Makefile targets for compose up/down (optional convenience)
- Dockerfile for the Go router

Out of scope:

- Fastino cloud API
- Automatic fine-tune / redeploy of GLiNER
- GPU-required-only path (CPU must work; GPU optional via compose override later)

## Architecture

```
Client → router:8080 → classifier.GLiNER → http://gliner:8090/v1/classify
                              └ on error → heuristic
```

## GLiNER sidecar API

### `GET /health`

```json
{"ok": true, "model": "fastino/GLiNER2.5-Decide", "ready": true}
```

### `POST /v1/classify`

Request:

```json
{
  "text": "fix the typo in main.go",
  "tasks": {
    "complexity": {
      "labels": ["trivial", "simple", "medium", "hard"]
    },
    "needs_tools": {
      "labels": ["yes", "no"]
    }
  }
}
```

Response:

```json
{
  "results": {
    "complexity": {
      "label": "simple",
      "confidence": 0.81,
      "probabilities": {"trivial": 0.05, "simple": 0.81, "medium": 0.12, "hard": 0.02}
    },
    "needs_tools": {
      "label": "yes",
      "confidence": 0.72,
      "probabilities": {"yes": 0.72, "no": 0.28}
    }
  }
}
```

Model loads once at startup. Env: `GLINER_MODEL` (default `fastino/GLiNER2.5-Decide`), `PORT` (default `8090`).

## Router changes

| Piece | Change |
|-------|--------|
| `RuntimeSettings` | `Classifier` default `gliner`; add `GLiNERBaseURL` (default `http://127.0.0.1:8090` locally / `http://gliner:8090` in compose) |
| `Server.classifier()` | case `gliner` → `NewGLiNER(...)` with heuristic fallback |
| `Result.Source` | `"gliner"` |
| Settings UI | options: GLiNER2.5 / nimble / heuristic; field for GLiNER base URL |
| Config example | document compose URLs |

## Docker Compose

Services:

1. **gliner** — build `services/gliner`, expose `8090`, volume `gliner-hf-cache:/root/.cache/huggingface`
2. **router** — build root Dockerfile, expose `8080`, env/settings pointing classifier at `http://gliner:8090`, mount config + data dir

Router waits for gliner health (compose `depends_on` + healthcheck).

## Success criteria

- `docker compose up --build` starts both services
- Settings default classifier is GLiNER2.5
- Chat routing calls GLiNER; on sidecar down, falls back to heuristic without failing the request path
- Unit test: GLiNER client against `httptest` mock of `/v1/classify`
