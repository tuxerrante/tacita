SHELL := /bin/bash

APP := tacita
BIN_DIR := bin
TOOLS_DIR := .go/bin
UV_TOOLS_DIR := .go/uv-tools
RUMDL_CACHE_DIR := .go/rumdl-cache
COVERAGE_FILE := coverage.out
MIN_COVERAGE ?= 80
FUZZTIME ?= 1m

RUMDL_VERSION ?= 0.2.60
RUMDL := $(TOOLS_DIR)/rumdl
RUMDL_STAMP := $(UV_TOOLS_DIR)/.rumdl-$(RUMDL_VERSION)

# The pinned Go tools are managed by bingo: each version lives in its own nested
# module under .bingo, and .bingo/Variables.mk exposes GOFUMPT, GITLEAKS,
# GOLANGCI_LINT, GOVULNCHECK, and BINGO as version-stamped paths whose targets
# rebuild whenever the matching .bingo/<tool>.mod changes. Point GOBIN at
# TOOLS_DIR before the include so those binaries land under .go/bin; the include
# leaves GOBIN untouched because it assigns with ?=.
GOBIN := $(CURDIR)/$(TOOLS_DIR)
include .bingo/Variables.mk

LINT_BASE ?= $(shell git merge-base HEAD origin/main 2>/dev/null || git rev-parse HEAD^ 2>/dev/null || git rev-parse HEAD 2>/dev/null)

.PHONY: all help tools fmt fmt-check markdown-fmt-check markdown-check markdown-gate vet lint lint-new test test-race fuzz coverage build check quality-gate security gitleaks pre-commit-install pre-commit-run clean
.NOTPARALLEL: check markdown-gate quality-gate pre-commit-run

all: check ## Run the incremental local validation gate

help: ## Show available targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		LC_ALL=C sort -t ':' -k1,1 | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

tools: $(GOFUMPT) $(GITLEAKS) $(GOLANGCI_LINT) $(GOVULNCHECK) $(RUMDL) $(TOOLS_DIR)/gofumpt $(TOOLS_DIR)/gitleaks ## Install pinned development tools locally

# bingo builds only the version-stamped binary (for example gofumpt-v0.11.0).
# pre-commit invokes tools by their bare .go/bin/<tool> name, so relink those
# names to the pinned build. The link depends on the versioned path, so a pin
# bump repoints it without a manual clean.
$(TOOLS_DIR)/gofumpt: $(GOFUMPT)
	@ln -sf "$(notdir $(GOFUMPT))" "$@"

$(TOOLS_DIR)/gitleaks: $(GITLEAKS)
	@ln -sf "$(notdir $(GITLEAKS))" "$@"

# Relink a bare bingo so maintainers can change a pin without hard-coding the
# bingo version; this is not part of `tools` because only pin changes need it.
$(TOOLS_DIR)/bingo: $(BINGO)
	@ln -sf "$(notdir $(BINGO))" "$@"

# rumdl ships as a PyPI wheel, not a Go module, so bingo cannot pin it. The
# install recipe lives on the binary target so a fresh checkout or a deleted
# binary always reinstalls; the version-stamped prerequisite adds the
# reinstall-on-bump behaviour, since bumping RUMDL_VERSION names a stamp that
# does not exist yet and is therefore newer than the binary. The recipe touches
# the stamp before installing so the freshly written binary is the newest file,
# which keeps an unchanged pin a no-op instead of reinstalling on every run.
$(RUMDL): $(RUMDL_STAMP)
	@command -v uv >/dev/null || { echo "uv is required to install rumdl" >&2; exit 1; }
	@mkdir -p "$(TOOLS_DIR)" "$(UV_TOOLS_DIR)"
	@rm -f "$(UV_TOOLS_DIR)"/.rumdl-* && touch "$(RUMDL_STAMP)"
	UV_TOOL_DIR="$(CURDIR)/$(UV_TOOLS_DIR)" \
		UV_TOOL_BIN_DIR="$(CURDIR)/$(TOOLS_DIR)" \
		uv tool install --force "rumdl==$(RUMDL_VERSION)"

$(RUMDL_STAMP):
	@mkdir -p "$(UV_TOOLS_DIR)" && touch "$@"

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

markdown-fmt-check: $(RUMDL) ## Fail when Markdown is not formatted
	"$(RUMDL)" fmt --check .

markdown-check: $(RUMDL) ## Lint Markdown
	"$(RUMDL)" check .

markdown-gate: markdown-fmt-check markdown-check ## Validate a Markdown-only change

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
