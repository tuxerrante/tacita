SHELL := /bin/bash

APP := tacita
BIN_DIR := bin
TOOLS_DIR := .go/bin
COVERAGE_FILE := coverage.out
MIN_COVERAGE ?= 80

GOLANGCI_LINT_VERSION ?= v2.6.0
GOVULNCHECK_VERSION ?= v1.1.4
GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint
GOVULNCHECK := $(TOOLS_DIR)/govulncheck

LINT_BASE ?= $(shell git merge-base HEAD origin/main 2>/dev/null || git rev-parse HEAD^ 2>/dev/null || git rev-parse HEAD 2>/dev/null)

.PHONY: all help tools fmt fmt-check vet lint lint-new test test-race coverage build check quality-gate security clean
.NOTPARALLEL: check quality-gate

all: check ## Run the incremental local validation gate

help: ## Show available targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		LC_ALL=C sort -t ':' -k1,1 | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

tools: $(GOLANGCI_LINT) $(GOVULNCHECK) ## Install pinned development tools locally

$(GOLANGCI_LINT):
	@mkdir -p "$(TOOLS_DIR)"
	GOBIN="$(CURDIR)/$(TOOLS_DIR)" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOVULNCHECK):
	@mkdir -p "$(TOOLS_DIR)"
	GOBIN="$(CURDIR)/$(TOOLS_DIR)" go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

fmt: ## Format Go source in place
	gofmt -s -w .

fmt-check: ## Fail when Go source is not formatted
	@files="$$(gofmt -s -l .)"; \
	if [[ -n "$$files" ]]; then \
		echo "The following files need gofmt:"; \
		echo "$$files"; \
		exit 1; \
	fi

vet: ## Run the standard Go analyzer
	go vet ./...

lint: $(GOLANGCI_LINT) ## Run the full pinned linter set
	"$(GOLANGCI_LINT)" run ./...

lint-new: $(GOLANGCI_LINT) ## Lint changes since LINT_BASE, or everything before the first commit
	@if [[ -n "$(LINT_BASE)" ]]; then \
		echo "Linting changes since $(LINT_BASE)"; \
		"$(GOLANGCI_LINT)" run --new-from-rev="$(LINT_BASE)" ./...; \
	else \
		echo "No Git baseline found; running the full linter set"; \
		"$(GOLANGCI_LINT)" run ./...; \
	fi

test: ## Run unit and integration tests
	go test -shuffle=on ./...

test-race: ## Run tests with the race detector
	go test -race -shuffle=on ./...

coverage: ## Enforce the minimum test coverage
	go test -race -covermode=atomic -coverprofile="$(COVERAGE_FILE)" ./...
	@coverage="$$(go tool cover -func="$(COVERAGE_FILE)" | awk '/^total:/ {sub(/%/, "", $$3); print $$3}')"; \
	if [[ -z "$$coverage" ]]; then \
		echo "Unable to determine total coverage"; \
		exit 1; \
	fi; \
	awk -v coverage="$$coverage" -v minimum="$(MIN_COVERAGE)" 'BEGIN { \
		printf "Total coverage: %.1f%% (minimum: %.1f%%)\n", coverage, minimum; \
		if (coverage < minimum) exit 1; \
	}'

build: ## Build a reproducible local binary
	@mkdir -p "$(BIN_DIR)"
	CGO_ENABLED=0 go build -trimpath -o "$(BIN_DIR)/$(APP)" ./cmd/$(APP)

check: fmt-check vet lint-new test ## Run fast incremental checks

quality-gate: fmt-check vet lint test-race coverage build security ## Run all pre-publication checks

security: $(GOVULNCHECK) ## Check dependencies and reachable code for known vulnerabilities
	"$(GOVULNCHECK)" ./...

clean: ## Remove generated local artifacts
	rm -rf -- "$(BIN_DIR)" "$(TOOLS_DIR)"
	rm -f -- "$(COVERAGE_FILE)"
