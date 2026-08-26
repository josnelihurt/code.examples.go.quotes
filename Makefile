# Developer entry points. CI runs the same commands (build/test/lint), so a
# green laptop means a green build.

.PHONY: help build test lint tidy

help: ## Show the available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

build: ## Compile every package
	go build ./...

test: ## Run every test with the race detector and coverage
	go test ./... -race -coverprofile=coverage.out -covermode=atomic

lint: ## Run the committed golangci-lint configuration
	golangci-lint run

tidy: ## Sync go.mod and go.sum with the current imports
	go mod tidy
