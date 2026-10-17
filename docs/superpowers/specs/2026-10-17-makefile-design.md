# Makefile Design

Date: 2026-10-17  
Status: approved for implementation

## Goal

Add a root `Makefile` so common Go workflows for this router project are discoverable via `make help` / `make about` and runnable via `make build`, `make run`, and `make test`.

## Scope

In scope:

- Root `Makefile` with targets: `help`, `about`, `build`, `run`, `test`
- Root `.gitignore` entry for `bin/` (no `.gitignore` exists today)

Out of scope:

- README changes
- Extra targets (`clean`, `fmt`, `lint`, version stamping)
- Taskfile / Just / other runners

## Decisions

| Topic | Choice |
|-------|--------|
| Style | Minimal Makefile with `.PHONY` and `##` self-docs |
| Default goal | `help` |
| `run` | `go run ./cmd/router` (does not require `build`) |
| `build` | Produce `bin/router` from `./cmd/router` |
| Config for `run` | Variable `CONFIG`, default `configs/config.example.yaml` |
| Tests | `go test ./...` |

## Targets

### `help` (default)

Print available targets and one-line descriptions, parsed from `##` comments next to recipes.

### `about`

Print short project identity useful for newcomers:

- Name: OpenAI Router Proxy
- Go module: `router`
- Binary path: `bin/router`
- Default config path used by `run`

### `build`

```text
mkdir -p bin
go build -o bin/router ./cmd/router
```

### `run`

```text
go run ./cmd/router -config $(CONFIG)
```

Override example: `make run CONFIG=configs/my.yaml`

### `test`

```text
go test ./...
```

## Variables

| Name | Default | Purpose |
|------|---------|---------|
| `BINARY` | `bin/router` | Output path for `build` |
| `CMD` | `./cmd/router` | Package / main package path |
| `CONFIG` | `configs/config.example.yaml` | Config passed to `run` |

## Companion change

Create `.gitignore` containing at least:

```gitignore
/bin/
```

## Success criteria

- `make` / `make help` lists the five targets with descriptions
- `make about` prints identity info without building
- `make build` creates `bin/router`
- `make run` starts the proxy via `go run` with the default example config
- `make test` runs the full Go test suite
- Built binaries under `bin/` are ignored by git
