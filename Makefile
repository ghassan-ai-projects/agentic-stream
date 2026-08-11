# ---------------------------------------------------------------------------
# Agentic Stream — Makefile
# ---------------------------------------------------------------------------
# Run `make help` to list all targets.
# Run `make ci-check` to run the full CI pipeline locally.
# ---------------------------------------------------------------------------

# ---- Configurable ---------------------------------------------------------
BINARY    ?= bin/$(shell basename $(CURDIR))
MODULE    ?= github.com/ghassan-ai-projects/agentic-stream
VERSION   := $(shell git describe --tags 2>/dev/null || echo dev)
COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS   := -ldflags="-X main.Version=$(VERSION) -X main.Commit=$(COMMIT)"

# Detect if any Go packages exist (after the user adds code). Used to skip
# targets gracefully in a freshly-cloned template.
PKGS := $(shell go list ./... 2>/dev/null)
HAS_PKGS := $(if $(PKGS),yes,no)

# Detect main packages separately. doc.go keeps one non-main package at the
# module root, so HAS_PKGS is yes even in a fresh template -- but deadcode
# needs a main package as its entry point and errors out without one.
MAIN_PKGS := $(shell go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./... 2>/dev/null)
HAS_MAIN := $(if $(MAIN_PKGS),yes,no)

# Default target
.DEFAULT_GOAL := help

# ---- Phony declarations ---------------------------------------------------
.PHONY: help all build vet fmt tidy lint lint-ci test test-short test-race \
        test-coverage ci-check deadcode vulncheck clean run cross-compile

# ---- Help -----------------------------------------------------------------
help: ## Show this help message
	@echo "Available targets:"
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ---- Build ----------------------------------------------------------------
all: build test lint ## Build, test, and lint (default full pipeline)

build: ## Compile all packages
	@if [ -d cmd ]; then \
	  go build $(LDFLAGS) -o $(BINARY) ./cmd/...; \
	else \
	  echo "(no cmd/ directory yet -- add cmd/<name>/main.go to enable build)"; \
	fi

cross-compile: ## Cross-compile linux/amd64 binary
	@if [ -d cmd ]; then \
	  mkdir -p bin/; \
	  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) \
	    -o bin/$(shell basename $(BINARY))-linux-amd64 ./cmd/...; \
	  echo "Linux binary: bin/$(shell basename $(BINARY))-linux-amd64 ($$(ls -lh bin/$(shell basename $(BINARY))-linux-amd64 | awk '{print $$5}'))"; \
	else \
	  echo "(no cmd/ directory yet -- nothing to cross-compile)"; \
	fi

run: ## Run the binary (requires cmd/<name>/main.go)
	@if [ -d cmd ]; then \
	  go run ./cmd/... $(ARGS); \
	else \
	  echo "ERROR: no cmd/ directory found. Create cmd/<name>/main.go first."; \
	  exit 1; \
	fi

# Usage: make run ARGS="--flag value"
ARGS ?=

# ---- Quality --------------------------------------------------------------
vet: ## Run go vet
	@if [ "$(HAS_PKGS)" = "yes" ]; then \
	  go vet $(PKGS); \
	else \
	  echo "(no packages yet -- skipping vet)"; \
	fi

fmt: ## Format code with goimports
	@if command -v goimports >/dev/null 2>&1; then \
	  goimports -w -local $(MODULE) .; \
	else \
	  gofmt -w .; \
	  echo "(install goimports for import-grouping: go install golang.org/x/tools/cmd/goimports@latest)"; \
	fi

tidy: ## Run go mod tidy
	go mod tidy

lint: ## Run golangci-lint
	golangci-lint run --timeout=3m

lint-ci: ## Run golangci-lint with full timeout (for CI)
	golangci-lint run --timeout=5m

# ---- Test -----------------------------------------------------------------
test: ## Run all tests with race + shuffle + coverage
	@if [ "$(HAS_PKGS)" = "yes" ]; then \
	  go test -race -count=1 -shuffle=on -coverprofile=coverage.out $(PKGS) && \
	  (go tool cover -func=coverage.out 2>/dev/null | grep total || true); \
	else \
	  echo "(no packages yet -- skipping test)"; \
	fi

test-short: ## Run tests in short mode (skip integration)
	@if [ "$(HAS_PKGS)" = "yes" ]; then \
	  go test -short -race -count=1 -shuffle=on $(PKGS); \
	else \
	  echo "(no packages yet -- skipping test)"; \
	fi

test-race: ## Run tests with race detector
	@if [ "$(HAS_PKGS)" = "yes" ]; then \
	  go test -race -count=1 $(PKGS); \
	else \
	  echo "(no packages yet -- skipping test)"; \
	fi

test-coverage: ## Run tests and produce HTML coverage report
	@if [ "$(HAS_PKGS)" = "yes" ]; then \
	  go test -coverprofile=coverage.out $(PKGS) && \
	  go tool cover -html=coverage.out -o coverage.html && \
	  echo "Coverage: coverage.html"; \
	else \
	  echo "(no packages yet -- skipping coverage)"; \
	fi

# ---- Pipeline -------------------------------------------------------------
ci-check: tidy build vet lint-ci test-short deadcode vulncheck ## Run the full CI pipeline locally (matches .github/workflows/ci.yml)
	@echo "  CI check passed"

# ---- Tools ----------------------------------------------------------------
deadcode: ## Detect unused exported functions
	@if ! command -v deadcode >/dev/null 2>&1; then \
	  echo "(deadcode not installed -- run: go install golang.org/x/tools/cmd/deadcode@latest)"; \
	elif [ "$(HAS_MAIN)" != "yes" ]; then \
	  echo "(no main package yet -- skipping deadcode)"; \
	else \
	  deadcode -test ./...; \
	fi

vulncheck: ## Run govulncheck
	@if command -v govulncheck >/dev/null 2>&1; then \
	  if [ "$(HAS_PKGS)" = "yes" ]; then \
	    govulncheck ./...; \
	  else \
	    echo "(no packages yet -- skipping vulncheck)"; \
	  fi; \
	else \
	  echo "(govulncheck not installed -- run: go install golang.org/x/vuln/cmd/govulncheck@latest)"; \
	fi

# ---- Cleanup --------------------------------------------------------------
clean: ## Remove build artifacts
	rm -f coverage.out coverage.html
	rm -rf bin/
