# Tacita Implementation Plan

Status: Milestone 0 contract re-frozen; Milestone 1 ingestion complete

Active milestone: Milestone 2 — descriptive miner

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

Product ratification, product manifests, and enforcement begin only after the
experiment passes its frozen gates. Experiment review manifests are evidence
artifacts, not product policy.

## Current implementation

Implemented:

- a Go 1.27 module with zero runtime dependencies;
- a testable `tacita backtest` command shell;
- process-boundary exit codes and signal cancellation;
- in-memory CLI tests;
- supported Linux/amd64, Git 2.43+, and SHA-1 validation;
- bounded, isolated revision resolution to a full commit object ID;
- a run-scoped repository value that fails closed unless its constructor
  validated the environment and classified the repository target;
- rejection of shallow, grafted, alternate-backed, and promisor repositories
  before any history command can run;
- bounded Git output that stops the child at its limit;
- streamed first-parent event metadata, validated as a chain while it is read;
- deterministic classification of unavailable objects found by first-parent
  commit traversal, with conservative fallback for unclassifiable Git failures;
- streamed `diff-tree` records normalized into per-event changed paths, with
  disjoint exclusion counts and no cross-event accumulation;
- lexical path-to-component projection into bounded, deduplicated transactions;
- repository-wide distinct path and component identity budgets enforced before
  transaction visitors observe an over-limit event;
- real-repository coverage for bare repositories, hostile revisions, unusual
  paths, ambient Git isolation, unsupported formats, and cancellation;
- incremental and full repository quality gates.

Not implemented:

- transaction aggregation;
- mining, baselines, or temporal evaluation;
- profile evaluation;
- report schema or rendering;
- proposal, ratification, manifests, or checking.

The current CLI is disposable. Its shape may change when the first real command
contract is frozen. The implemented Git boundary is likewise provisional and
still under review; each review pass so far has found and fixed a defect,
recorded in [`docs/architecture.md`](docs/architecture.md#repository-targeting).

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

The dated amendment record in
[`docs/experiment.md`](docs/experiment.md#freeze-record) documents corrections
made after human review of public history but before Tacita produced Kapparmor
candidate or metric output. The experiment is re-frozen; later changes require
a newly preregistered run. Kapparmor results cannot modify this table.

## Next implementation steps

Build Milestone 2 as two dependency-safe pull requests with one motivation
each. Both operate on synthetic transaction sequences and neither reads the
development or holdout corpus:

1. add the pure `internal/mining` accumulation fold: intern component
   identities, maintain component-keyed opportunity and weighted occurrence
   sums, maintain pair-keyed raw and weighted support, compute all three frozen
   size weights and their run-wide eligible-transaction totals side by side,
   and enforce the directional-observation and distinct-pair budgets with typed
   errors;
2. derive finite descriptive metrics from one completed aggregate, abstain
   below 100 eligible transactions, apply the fixed raw-support floor and all
   81 post-aggregation configurations, and rank eligible candidates with the
   frozen byte-level tie-breakers.

The first pull request owns accumulation and resource exhaustion. The second
owns candidate eligibility and ordering. Neither introduces temporal cutoffs,
baselines, controls, report encoding, CLI wiring, concurrency, or runtime
dependencies.

Development calibration is not a Milestone 2 shortcut over final-history
aggregates. Its selector requires temporal partitions, baselines, controls, and
canonical reports, so the development lock is produced in Milestone 3 before
any holdout report. Moving that work changes only the operational milestone
boundary; the frozen grid, selector, corpus, report, and holdout rules remain
unchanged.

## Design review records

### Milestone 1 design review record

A pre-implementation review, repeated with a second independent model, changed
the design. It ran against Git 2.55.0 and recorded these outcomes:

| Question | Outcome |
| --- | --- |
| Does `git -C <path>` confine Git to that path? | No. A non-repository path resolved an ancestor repository's commit, so discovery is removed rather than detected. |
| Are `GIT_CEILING_DIRECTORIES` or `GIT_DISCOVERY_ACROSS_FILESYSTEM` enough? | No. The first cannot express a path containing `:`; the second only stops mount crossings. |
| Can the repository be identified by comparing `--git-dir` to the supplied path? | No. Bare repositories, linked worktrees, submodules, and symlinked paths each report a different directory. Explicit targeting avoids the comparison. |
| Does revision resolution mutate a partial clone? | No. `rev-parse` left the object store unchanged; `diff-tree` fetched and wrote new packs. Preflight must precede traversal, not resolution. |
| Is `rev-list --parents` output bounded per line? | No. An octopus merge printed all five parents on one line under `--first-parent`, so the stream must be parsed incrementally. |
| Is streaming `diff-tree` with an in-memory stdin deadlock-free? | Only on the success path. Early termination must cancel, drain to EOF, then wait. |
| Can preflight prove complete local objects? | No. The frozen flow omits `--root` and never traverses secondary parents, so missing objects are classified during traversal instead. |

These are implementation and safety corrections. They change no corpus ID,
threshold, report field, resource budget, or component or history semantics, so
the Milestone 0 freeze is unaffected.

### Milestone 2 design review record

Two independent pre-implementation reviews found that the original direct path
from aggregation to a development lock omitted required temporal and reporting
work. They recorded these outcomes:

| Question | Outcome |
| --- | --- |
| Can final-history aggregates select the development lock? | No. The frozen selector requires cutoff adherence, frequent-consequent comparison, coverage, adjacent-cutoff Jaccard, controls, and canonical development reports. |
| Does an eligible transaction's stream position equal its integration event index? | No. Cutoffs are defined over every first-parent event before exclusions, so Milestone 3 must map transaction object IDs to the complete event sequence. |
| Should `internal/gitlog.Transaction` gain an event index? | No. Milestone 1 is complete, and the full event sequence already lets `internal/backtest` own that composition without reopening the ingestion boundary. |
| Must Milestone 2 retain and shuffle transactions? | No. Replay over the hash-shuffled order is a Milestone 3 control; the Milestone 2 fold remains a pure function of one supplied sequence. |
| Are component exposure and prevalence separate accumulated values? | No. For each weight mode, one component occurrence sum is used as antecedent exposure and consequent prevalence numerator. |
| Does the raw-support floor reduce pair budget accounting? | No. Observation and distinct-pair budgets apply while folding; raw support of at least three is a later candidate-eligibility filter. |

These findings split independently testable responsibilities and correct the
milestone boundary. They change no frozen metric, threshold, configuration,
corpus ID, report field, resource budget, or Git or component semantics.

## Milestones

### 0. Freeze the experiment — complete 2026-08-16

Deliver:

- resolved blocking decisions;
- a threat model and explicit exclusions;
- an experiment another implementer can reproduce without hidden choices.

Exit met: all inputs and acceptance criteria are fixed before holdout results
are observed.

### 1. Safe Git ingestion — complete 2026-08-25

Deliver:

- cancellation-aware Git runner;
- safe revision resolution;
- NUL-delimited history parser;
- normalized transactions and exclusion diagnostics;
- integration and fuzz coverage for adversarial repositories.

Exit: deterministic bounded output, complete child-process cleanup, and explicit
incomplete-history behavior.

Revision resolution, preflight against known incompleteness mechanisms, and
streamed history traversal are complete. Missing objects exposed by
first-parent commit traversal are classified from machine-readable Git output;
unclassifiable traversal failures remain conservative Git failures. Component
projection from normalized event paths into bounded, deduplicated component
transactions is also implemented, and that stream owns the frozen global path
and component identity budgets. Milestone 1 implementation is complete.

### 2. Descriptive miner

Starting from normalized component transactions, deliver directional pair
aggregation, raw and weighted measures, stable ranking, all 81 frozen
configurations, tests, and benchmarks.

Aggregation follows the single-pass
[aggregation strategy](docs/architecture.md#aggregation-strategy): one ordered
fold with component-keyed and pair-keyed state, three weighted sums, and the
grid derived by post-aggregation filtering. There is no in-process fan-out.

Exit: exact repeated full-history aggregates and ranked candidates for every
configuration, bounded behavior, and no development or holdout corpus access.

### 3. Temporal calibration and backtest

Deliver the frozen rolling cutoffs, trivial baselines, negative and stress
controls, canonical analytical reports, development calibration and
configuration lock, blinded human review, and the configuration holdout
evidence bundle.

The four cutoffs are snapshots of the same fold, not four runs. The
hash-shuffled control reuses that fold over its own event ordering.

The development lock must exist before any holdout report is generated. Exit:
a complete go/no-go result without holdout-informed configuration or threshold
changes.

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
