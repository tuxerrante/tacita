# Tacita Implementation Plan

Status: Milestone 0 experiment contract frozen

Active milestone: Milestone 1 — safe Git ingestion

Product implementation: limited to the frozen evidence-first experiment

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

## Frozen Milestone 0 decisions

Milestone 1 must implement these contracts without changing them from
development or holdout results:

| Decision | Required output |
| --- | --- |
| [Evaluation corpus](docs/experiment.md#frozen-evaluation-corpus) | Frozen: repository roles and immutable commit IDs |
| [Product gate](docs/experiment.md#frozen-product-relevance-gate) | Frozen: blinded review protocol and numeric precision thresholds |
| [Integration history](docs/architecture.md#integration-history) | Frozen: first-parent events, net merge diffs, and merge-policy limits |
| [Temporal weighting](docs/experiment.md#descriptive-model) | Frozen: no temporal decay in the first experiment |
| [Temporal backtest](docs/experiment.md#temporal-evaluation) | Frozen: expanding 60/70/80/90 percent cutoffs and non-overlapping 10% windows |
| [Gate outcomes](docs/experiment.md#gate-outcome-semantics) | Frozen: conjunctive pass, fail, inconclusive, and undefined metrics |
| [Candidate calibration](docs/experiment.md#candidate-configuration-grid) | Frozen: 81 configurations, selector, ranking, and development lock |
| [Engineering gate](docs/experiment.md#frozen-engineering-gate) | Frozen: determinism, controls, adherence, coverage, stability, and cost |
| [Report contract](docs/report-v1.md) | Frozen: report, development lock, review manifest, sidecar, encoding, and ordering |
| [Git boundary](docs/architecture.md#supported-experiment-environment) | Frozen: Linux/amd64, Git 2.43+, SHA-1, complete local objects, and no rename inference |
| [Component projection](docs/architecture.md#component-projection) | Frozen: lexical parent directories, root, modules, gitlinks, vendor, and symlinks |
| [Resource budgets](docs/architecture.md#resource-limits) | Frozen: events, paths, components, pairs, configurations, bytes, time, memory, and rows |
| [Profiles](docs/profiles.md) | Frozen: parser, applicability, identifiers, rule semantics, and provenance |
| [Threat model](docs/threat-model.md) | Frozen: assets, trust boundaries, controls, residual risks, and exclusions |

The experiment may be revised only as a newly preregistered run. Results from
Kapparmor cannot modify this table.

## Next implementation step

Begin Milestone 1 with the smallest independently testable Git boundary:

1. implement supported-environment validation and full commit resolution;
2. add the cancellation-aware bounded Git runner;
3. parse first-parent integration events and exclusion diagnostics;
4. validate against real temporary repositories before adding mining.

Do not implement candidate aggregation, calibration, profiles, or holdout
evaluation in Milestone 1.

## Milestones

### 0. Freeze the experiment — complete 2026-08-16

Deliver:

- resolved blocking decisions;
- a threat model and explicit exclusions;
- an experiment another implementer can reproduce without hidden choices.

Exit met: all inputs and acceptance criteria are fixed before holdout results
are observed.

### 1. Safe Git ingestion

Deliver:

- cancellation-aware Git runner;
- safe revision resolution;
- NUL-delimited history parser;
- normalized transactions and exclusion diagnostics;
- integration and fuzz coverage for adversarial repositories.

Exit: deterministic bounded output, complete child-process cleanup, and explicit
incomplete-history behavior.

The first learning exercise should implement `ResolveCommit` under the frozen
environment, history, error, and budget contracts.

### 2. Descriptive miner

Deliver component projection, directional pair aggregation, raw and weighted
measures, stable ranking, the frozen development grid, tests, and benchmarks.

Exit: exact repeated results, bounded behavior, and a configuration lock
selected from the development corpus without holdout access.

### 3. Temporal backtest

Deliver the frozen rolling cutoffs and configuration, trivial baselines,
blinded human review, negative and stress controls, and the untouched holdout
evidence bundle.

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
