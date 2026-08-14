# Tacita Implementation Plan

Status: specification and disposable CLI bootstrap

Active milestone: Milestone 0 — freeze the experiment

Product implementation: blocked on the decisions below

This file is the operational plan. It intentionally does not repeat the product
definition, experimental protocol, or architecture:

- [`docs/product.md`](docs/product.md) defines the product boundary;
- [`docs/experiment.md`](docs/experiment.md) defines what evidence can validate
  or falsify the product hypothesis;
- [`docs/architecture.md`](docs/architecture.md) defines implementation,
  security, and testing contracts.

## Goal

Build the smallest evidence-first vertical slice that can answer:

> Can Tacita surface stable, explainable candidate conventions that maintainers
> recognize as intentional and worth checking?

The first slice ends at deterministic proposal reports:

```text
Git history
  -> normalized component transactions
  -> directional candidate conventions
  -> temporal and product-relevance evaluation
  -> deterministic reports
```

Ratification, manifests, and enforcement begin only after the experiment passes
its frozen gates.

## Current implementation

Implemented:

- a Go 1.26 module with zero runtime dependencies;
- a testable `tacita backtest` command shell;
- process-boundary exit codes and signal cancellation;
- in-memory CLI tests;
- incremental and full repository quality gates.

Not implemented:

- Git process execution or history parsing;
- transaction normalization or component projection;
- mining, baselines, or temporal evaluation;
- profile evaluation;
- report schema or rendering;
- proposal, ratification, manifests, or checking.

The current CLI is disposable. Its shape may change when the first real command
contract is frozen.

## Blocking decisions

Milestone 1 must not invent these values while coding:

| Decision | Required output |
| --- | --- |
| Evaluation corpus | Repository roles and immutable commit IDs |
| Product gate | Blinded review protocol and numeric precision thresholds |
| Engineering gate | Frozen adherence, coverage, stability, runtime, and memory thresholds |
| Report contract | Versioned schema v1 and canonical ordering rules |
| Git semantics | Supported platforms/Git version, history ordering, shallow-history and rename behavior |
| Component projection | Root, deleted-only directories, nested modules, submodules, vendor, and symlink behavior |
| Resource budgets | Commits, paths, components, pairs, bytes, time, and report rows |
| Profiles | Exact parser semantics, applicability inputs, identifiers, and pinned provenance |

The provisional choices and their rationale are in
[`docs/experiment.md`](docs/experiment.md). A decision is frozen only when this
table can link to a concrete value or schema.

## Milestones

### 0. Freeze the experiment

Deliver:

- resolved blocking decisions;
- a threat model and explicit exclusions;
- an experiment another implementer can reproduce without hidden choices.

Exit: all inputs and acceptance criteria are fixed before holdout results are
observed.

### 1. Safe Git ingestion

Deliver:

- cancellation-aware Git runner;
- safe revision resolution;
- NUL-delimited history parser;
- normalized transactions and exclusion diagnostics;
- integration and fuzz coverage for adversarial repositories.

Exit: deterministic bounded output, complete child-process cleanup, and explicit
incomplete-history behavior.

The first learning exercise may implement `ResolveCommit` because its safety
contract is already known, but it must not silently choose the unresolved
history or budget semantics.

### 2. Descriptive miner

Deliver component projection, directional pair aggregation, raw and weighted
measures, stable ranking, tests, and benchmarks.

Exit: exact repeated results and bounded behavior on the development corpus.

### 3. Temporal backtest

Deliver rolling cutoffs, trivial baselines, blinded human review, negative and
stress controls, and the untouched holdout evidence bundle.

Exit: a complete go/no-go result without post-holdout threshold changes.

### 4. Cold-start profile evaluation

Deliver independently opt-in `go-core` and `spf13-idiomatic` candidate
evaluation with rule-level provenance and a time-boxed ratification study.

Exit: at least three useful profile-origin expectations can be ratified within
the frozen adoption budget without presenting them as repository-inferred.

### 5. Human product decision

Choose one:

1. plan proposal, ratification, and diff checking because both required lanes
   passed;
2. preregister and rerun a revised failed lane;
3. stop or split the product because the evidence does not support one coherent
   workflow.

Enforcement never starts automatically.

## Deferred roadmap

After a positive Milestone 5 decision, consider in order:

1. manifest and ratification model;
2. explicit-diff checking and baseline semantics;
3. optional provider-specific CI presentation;
4. additional opt-in profiles;
5. separately specified import-graph rules;
6. TUI over the versioned report model;
7. optional MCP integration isolated from the offline core;
8. generated documentation from ratified policy;
9. release automation and, only if justified, a Git-capable OCI image.

Kubernetes policy, opaque model scoring, network/LLM dependencies in analysis,
and authorship detection are outside the product boundary.

## Change discipline

- Update this file when milestone status, blockers, or implementation order
  changes.
- Update `docs/experiment.md` when evidence or acceptance semantics change.
- Update `docs/architecture.md` when an implementation invariant changes.
- Do not copy the same decision into multiple files.
- Repository publication and any remote operation require explicit approval.
