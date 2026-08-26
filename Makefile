# Developer entry points. CI runs the same commands (build/test/lint), so a
# green laptop means a green build.

.PHONY: help build test test-db bdd lint tidy contracts-go

# The podman machine's API socket, when one is running — the database
# integration tests need a container runtime and otherwise skip themselves.
# Ryuk cannot run on podman; the suite's TestMain detects that and owns the
# container teardown itself.
PODMAN_SOCKET := $(shell podman machine inspect --format '{{ .ConnectionInfo.PodmanSocket.Path }}' 2>/dev/null)

help: ## Show the available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

build: ## Compile every package
	go build ./...

test: ## Run every test with the race detector and coverage
	go test ./... -race -coverprofile=coverage.out -covermode=atomic

test-db: ## Run the database integration tests against a local container runtime
	DOCKER_HOST='$(if $(PODMAN_SOCKET),unix://$(PODMAN_SOCKET),$(DOCKER_HOST))' \
		go test ./internal/quotes/infrastructure/... -race -count=1

bdd: ## Run the specification suite against the compose stack (scripts/bdd.sh)
	./scripts/bdd.sh

lint: ## Run the committed golangci-lint configuration
	golangci-lint run

tidy: ## Sync go.mod and go.sum with the current imports
	go mod tidy

contracts-go: ## Regenerate the v3 Go contract code (pinned plugins; see contracts/quotes/v3/buf.gen.go.yaml)
	cd contracts/quotes/v3 && buf generate --template buf.gen.go.yaml --path quotes_v3.proto
