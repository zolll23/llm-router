# OpenAI Router Proxy

Go OpenAI-compatible proxy that routes requests across providers (Ollama, OpenAI-compatible, OpenCode config, Cursor CLI inventory) using a classifier and local-first policy.

See:

- Design: [`docs/superpowers/specs/2026-10-17-openai-router-proxy-design.md`](docs/superpowers/specs/2026-10-17-openai-router-proxy-design.md)
- Plan: [`docs/superpowers/plans/2026-10-17-openai-router-proxy.md`](docs/superpowers/plans/2026-10-17-openai-router-proxy.md)

## Quick start

```bash
go run ./cmd/router -config configs/config.example.yaml
curl -s http://127.0.0.1:8080/health
```

## OpenCode

Point an OpenAI-compatible provider at `http://127.0.0.1:8080/v1` and select model `auto` (documented further as features land).
