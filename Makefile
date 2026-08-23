SHELL := /bin/bash

APP := tacita
BIN_DIR := bin
TOOLS_DIR := .go/bin
UV_TOOLS_DIR := .go/uv-tools
RUMDL_CACHE_DIR := .go/rumdl-cache
COVERAGE_FILE := coverage.out
MIN_COVERAGE ?= 80
FUZZTIME ?= 1m

GOFUMPT_VERSION ?= v0.11.0
GITLEAKS_VERSION ?= v8.30.1
GOLANGCI_LINT_VERSION ?= v2.13.1
GOVULNCHECK_VERSION ?= v1.1.4
RUMDL_VERSION ?= 0.2.55
GOFUMPT := $(TOOLS_DIR)/gofumpt
GITLEAKS := $(TOOLS_DIR)/gitleaks
GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint
GOVULNCHECK := $(TOOLS_DIR)/govulncheck
RUMDL := $(TOOLS_DIR)/rumdl

LINT_BASE ?= $(shell git merge-base HEAD origin/main 2>/dev/null || git rev-parse HEAD^ 2>/dev/null || git rev-parse HEAD 2>/dev/null)

.PHONY: all help tools fmt fmt-check markdown-check vet lint lint-new test test-race fuzz coverage build check quality-gate security gitleaks pre-commit-install pre-commit-run clean
.NOTPARALLEL: check quality-gate pre-commit-run

all: check ## Run the incremental local validation gate

help: ## Show available targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		LC_ALL=C sort -t ':' -k1,1 | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

tools: $(GOFUMPT) $(GITLEAKS) $(GOLANGCI_LINT) $(GOVULNCHECK) $(RUMDL) ## Install pinned development tools locally

$(GOFUMPT):
	@mkdir -p "$(TOOLS_DIR)"
	GOBIN="$(CURDIR)/$(TOOLS_DIR)" go install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)

$(GITLEAKS):
	@mkdir -p "$(TOOLS_DIR)"
	GOBIN="$(CURDIR)/$(TOOLS_DIR)" go install github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION)

$(GOLANGCI_LINT):
	@mkdir -p "$(TOOLS_DIR)"
	GOBIN="$(CURDIR)/$(TOOLS_DIR)" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOVULNCHECK):
	@mkdir -p "$(TOOLS_DIR)"
	GOBIN="$(CURDIR)/$(TOOLS_DIR)" go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

$(RUMDL):
	@command -v uv >/dev/null || { echo "uv is required to install rumdl" >&2; exit 1; }
	@mkdir -p "$(TOOLS_DIR)" "$(UV_TOOLS_DIR)"
	UV_TOOL_DIR="$(CURDIR)/$(UV_TOOLS_DIR)" \
		UV_TOOL_BIN_DIR="$(CURDIR)/$(TOOLS_DIR)" \
		uv tool install --force "rumdl==$(RUMDL_VERSION)"

fmt: $(GOFUMPT) $(RUMDL) ## Format Go source and Markdown in place
	"$(GOFUMPT)" -w .
	"$(RUMDL)" fmt .

fmt-check: $(GOFUMPT) $(RUMDL) ## Fail when Go source or Markdown is not formatted
	@files="$$("$(GOFUMPT)" -l .)"; \
	if [[ -n "$$files" ]]; then \
		echo "The following files need gofumpt:"; \
		echo "$$files"; \
		exit 1; \
	fi
	"$(RUMDL)" fmt --check .

markdown-check: $(RUMDL) ## Lint Markdown
	"$(RUMDL)" check .

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

fuzz: ## Run every fuzz target against the mutation engine for FUZZTIME
	@fuzzed=0; \
	packages="$$(go list ./...)" || exit 1; \
	for package in $$packages; do \
		listing="$$(go test -list '^Fuzz' "$$package")" || exit 1; \
		for target in $$(echo "$$listing" | grep '^Fuzz'); do \
			echo "Fuzzing $$target in $$package for $(FUZZTIME)"; \
			go test "$$package" -run '^$$' -fuzz "^$$target\$$" -fuzztime "$(FUZZTIME)" || exit 1; \
			fuzzed=$$((fuzzed + 1)); \
		done; \
	done; \
	if [[ "$$fuzzed" -eq 0 ]]; then \
		echo "No fuzz target was found, so nothing was fuzzed" >&2; \
		exit 1; \
	fi

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

check: fmt-check markdown-check vet lint-new test ## Run fast incremental checks

quality-gate: fmt-check markdown-check vet lint test-race coverage build security ## Run all pre-publication checks

security: $(GOVULNCHECK) gitleaks ## Check for known vulnerabilities and hardcoded secrets
	"$(GOVULNCHECK)" ./...

gitleaks: $(GITLEAKS) ## Scan repository files for hardcoded secrets
	"$(GITLEAKS)" dir --no-banner --redact .

pre-commit-install: tools ## Install the repository Git pre-commit hook
	@command -v pre-commit >/dev/null || { echo "pre-commit is required to install hooks" >&2; exit 1; }
	pre-commit install

pre-commit-run: tools gitleaks ## Run the complete repository pre-commit suite
	@command -v pre-commit >/dev/null || { echo "pre-commit is required to run hooks" >&2; exit 1; }
	pre-commit run --all-files

clean: ## Remove generated local artifacts
	rm -rf -- "$(BIN_DIR)" "$(TOOLS_DIR)" "$(UV_TOOLS_DIR)" "$(RUMDL_CACHE_DIR)"
	rm -f -- "$(COVERAGE_FILE)"
