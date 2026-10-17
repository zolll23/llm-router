# GLiNER Docker Classifier Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run GLiNER2.5 as a local Docker sidecar and make it the default router classifier.

**Architecture:** FastAPI service in `services/gliner` exposes `/v1/classify`; Go `classifier.GLiNER` calls it with heuristic fallback; `docker-compose.yml` wires `gliner` + `router`.

**Tech Stack:** Go 1.26, FastAPI/uvicorn, `gliner2[local]`, Docker Compose, model `fastino/GLiNER2.5-Decide`.

## Global Constraints

- Default classifier: `gliner`
- Sidecar listen: `:8090`
- Fallback on GLiNER error: heuristic
- Spec: `docs/superpowers/specs/2026-10-17-gliner-docker-classifier-design.md`

---

## Task 1: GLiNER Go client + settings

**Files:**
- Create: `internal/classifier/gliner.go`, `internal/classifier/gliner_test.go`
- Modify: `internal/classifier/classifier.go`, `internal/api/settings.go`, `internal/api/server.go`, `internal/api/web/index.html`, `internal/api/web/app.js`

- [ ] Write httptest test for `/v1/classify` → complexity/needs_tools
- [ ] Implement `GLiNER` client with heuristic fallback
- [ ] Default settings to `gliner` + `GLiNERBaseURL`
- [ ] Wire UI select + save/load
- [ ] `go test ./internal/classifier/ ./internal/api/ -count=1`

## Task 2: GLiNER FastAPI sidecar

**Files:**
- Create: `services/gliner/app.py`, `services/gliner/requirements.txt`, `services/gliner/Dockerfile`

- [ ] Implement `/health` and `/v1/classify` using `AutoExtractor`
- [ ] Dockerfile with model download at build or first-start cache volume

## Task 3: Router Docker + compose + Makefile

**Files:**
- Create: `Dockerfile`, `docker-compose.yml`
- Modify: `Makefile`, `.gitignore`, `configs/config.example.yaml` (comments/env notes), `README.md` (brief)

- [ ] Multi-stage Go Dockerfile
- [ ] Compose services `gliner` + `router` with healthcheck
- [ ] `make docker-up` / `make docker-down`
