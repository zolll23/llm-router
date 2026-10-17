BINARY ?= bin/router
CMD    ?= ./cmd/router
CONFIG ?= configs/config.example.yaml

.PHONY: help about build build-relay run run-blue run-green relay switch-blue switch-green test docker-up docker-down docker-logs

.DEFAULT_GOAL := help

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

about: ## Show project identity
	@echo "OpenAI Router Proxy"
	@echo "  module:  router"
	@echo "  binary:  $(BINARY)"
	@echo "  config:  $(CONFIG)"
	@echo "  classifier default: gliner (docker: make docker-up)"

build: ## Build binary to bin/router
	@mkdir -p $(dir $(BINARY))
	go build -o $(BINARY) $(CMD)

build-relay: ## Build relay to bin/relay
	@mkdir -p $(dir bin/relay)
	go build -o bin/relay ./cmd/relay

relay: ## Run relay proxy
	go run ./cmd/relay/main.go

run-blue: ## Run router on port 8081
	go run $(CMD) -config $(CONFIG) -port :8081

run-green: ## Run router on port 8082
	go run $(CMD) -config $(CONFIG) -port :8082

switch-blue: ## Switch relay to blue (8081)
	@echo "8081" > .current_port

switch-green: ## Switch relay to green (8082)
	@echo "8082" > .current_port

test: ## Run all Go tests
	go test ./...

docker-up: ## Build and start router + GLiNER via compose
	docker compose up --build -d

docker-down: ## Stop compose stack
	docker compose down

docker-logs: ## Tail compose logs
	docker compose logs -f --tail=200
