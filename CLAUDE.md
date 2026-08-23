# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Authority and precedence

[`AGENTS.md`](AGENTS.md) is the canonical cross-client agent context; read it and
[`IMPLEMENTATION_PLAN.md`](IMPLEMENTATION_PLAN.md) before any change. Owned
documents win over this summary: [`docs/product.md`](docs/product.md) (scope),
[`docs/experiment.md`](docs/experiment.md) (mining, metrics, corpus, gates),
[`docs/architecture.md`](docs/architecture.md) (CLI, Git, packages, security,
tests), [`docs/profiles.md`](docs/profiles.md),
[`docs/report-v1.md`](docs/report-v1.md), and
[`docs/threat-model.md`](docs/threat-model.md). When a decision changes, edit its
owning document instead of copying it. If same-depth `AGENTS.md` and `CLAUDE.md`
disagree, flag it rather than guessing.

## What Tacita is

An experimental Go 1.26 CLI that mines repository-local change conventions,
asks maintainers to ratify the intentional ones, and reports when a later diff
deviates from those versioned contracts. It is **not** an AI detector and does
**not** recommend the next edit. The core is offline, provider-neutral,
read-only, and has zero runtime module dependencies (standard library only until
measured complexity justifies otherwise).

The checked-in binary is a disposable `tacita backtest` shell — it does not yet
read Git history, mine, evaluate, ratify, or enforce. Its command shape is not
frozen. Active work is **Milestone 1 (safe Git ingestion)**; do not implement
mining, aggregation, profiles, or holdout evaluation.

## Commands

Requires Go 1.26, Git, GNU Make, and `uv`. Pinned dev tools install under
`.go/bin` and are never runtime dependencies.

```bash
make check         # fast gate: fmt-check, markdown-check, vet, lint-new, test
make quality-gate  # full gate: adds race, coverage (>=80%), build, security
make fmt           # gofumpt + rumdl format in place
make test          # go test -shuffle=on ./...
make fuzz          # run every Fuzz* target for FUZZTIME (default 1m)
```

Run a single test:

```bash
go test ./cmd/tacita -run '^TestRun$' -count=1
```

Run the current shell:

```bash
go run ./cmd/tacita backtest --revision HEAD .
```

`lint-new` lints only changes since `LINT_BASE` (the merge-base with
`origin/main`). Do not commit generated `bin/`, `.go/`, or `coverage.out`.

## Architecture

Package layout is one level deep under `internal/` (a compiler-enforced
boundary, not a junk drawer — no `pkg`, `utils`, `helpers`, or architectural
layers):

- `cmd/tacita` — process boundary and dependency wiring. Call path is
  `main → realMain → run → runBacktest`. `main` only wires args/streams and calls
  `os.Exit`; `realMain` owns signal cancellation, error rendering, and exit-code
  mapping (2 for `*usageError`, 1 otherwise). Handlers take injected `io.Writer`
  and return errors — errors are printed **once**, at the CLI boundary.
- `internal/gitlog` — Git execution, parsing, normalization, diagnostics
  (the only implemented internal package so far).
- `internal/mining`, `internal/backtest`, `internal/report` — planned; create a
  package only when its responsibility becomes independently testable.

### The Git boundary is the critical invariant

Analyzed repositories are **untrusted input**. `internal/gitlog.Open` returns a
concrete, run-scoped `Repository` value that makes correct ordering
unrepresentable: no object access before environment validation and target
classification. Because Go cannot forbid a zero-value struct, every Git
invocation passes a binding step that rejects any `Repository` the constructor
did not produce (fail-closed).

Non-negotiable rules when touching Git code:

- Invoke Git with `exec.CommandContext` and an argument array — **never a shell**.
- Target the repository explicitly (`--git-dir` / `--work-tree` from path
  classification). `git -C <path>` is **not** confinement — Git walks upward and
  can resolve an ancestor repository (target confusion).
- Resolve revisions with `rev-parse --verify --end-of-options <rev>^{commit}`,
  validate a full 40-byte lowercase SHA-1, and use only that ID downstream.
- Preflight (before any history command) rejects shallow, grafted,
  alternate-backed, and promisor/partial-clone repositories — a partial clone
  would fetch over the network and write packs during `diff-tree`, breaking both
  offline and read-only guarantees.
- Stream `rev-list` and `diff-tree`; parse `diff-tree` as NUL records, never
  lines (paths may contain spaces, tabs, newlines, non-UTF-8 bytes). Nothing is
  buffered whole; all work and output are capped (see resource limits in
  architecture.md).
- On early stop (budget/grammar failure): cancel the child's context, drain
  stdout to EOF through the bounded reader, wait, then return the saved failure;
  the original context's failure wins.
- Build the child environment from an allowlist (`LC_ALL=C`, `TZ=UTC`,
  `GIT_OPTIONAL_LOCKS=0`, `GIT_NO_REPLACE_OBJECTS=1`, disabled system/global
  config, empty pager) and clear inherited `GIT_*` variables.

Supported environment is frozen: Linux/amd64, Git 2.43+, SHA-1 object format.
Model changes as **first-parent integration events** (merge diffed against first
parent; root recorded but excluded from mining); no rename inference.

### Determinism and errors

Reports must be byte-identical for identical inputs: sort map-derived output, use
stable tie-breakers, exclude wall-clock/process-local values, and keep runtime
observations in a separate operational sidecar. JSON is authoritative; text
renders from the same typed model. Preserve the `inferred` vs `profile`
candidate provenance distinction everywhere.

Wrap errors with `%w`, branch on error identity (never message text), and never
turn malformed input, cancellation, or exhausted budgets into success-shaped
empty results.

### Testing

Test the Git boundary against **real** temporary repositories under `t.TempDir()`
with the installed Git — do not mock Git. Use table-driven unit tests; fuzz the
NUL parser, path/status decoding, normalization, and projection. A fuzz target
asserts a property (not merely the absence of a panic) and carries a seed for
each rule it covers.

## Change discipline

- Never commit to `main`. Work on a task branch; deliver via pull request. Do not
  publish, add remotes, open PRs, or rewrite shared history without explicit
  approval.
- **One motivation per pull request.** Classify every planned file by motivation
  before implementing; if any needs a different rationale, split into separate or
  stacked PRs. Sharing a subsystem or milestone is not a shared motivation.
  Documentation may accompany code only when it documents that exact behavior.
- Run `/review-pr-hygiene` on the final diff before publishing; treat mixed
  concerns as blocking. Merge by squash. Request Copilot review on every PR and
  resolve every conversation before assessing merge readiness.
- Keep execution sequential until a benchmark proves a bottleneck.
- All committed docs, comments, and user-facing text are in English. Follow the
  documentation emoji vocabulary in [`CONTRIBUTING.md`](CONTRIBUTING.md): exactly
  one emoji at the start of each `##` heading; **never** emit emoji from Go
  source, program output, or commit titles.
