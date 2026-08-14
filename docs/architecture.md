# Architecture

This document defines technical invariants for the evidence-first experiment.
It does not select product thresholds or approve enforcement.

## Principles

- Offline and provider-neutral core.
- Read-only Git access at explicit revisions.
- Standard library first and zero runtime module dependencies initially.
- Deterministic reports and explicit work budgets.
- Concrete types before project-specific interfaces.
- Sequential execution until profiling justifies bounded concurrency.
- Errors returned through the stack and printed once at the CLI boundary.

## Current CLI

The implemented call path is:

```text
main -> realMain -> run -> runBacktest
```

- `main` only passes process arguments and streams to `realMain`, then calls
  `os.Exit`.
- `realMain` owns signal cancellation, error rendering, and exit-code mapping.
- `run` and `runBacktest` accept injected `io.Writer` values and return errors.
- a dedicated `flag.FlagSet` captures help and parse output so the CLI boundary
  controls where each message is written;
- `usageError` preserves its cause for `errors.Is` and `errors.As`.

The `backtest` shell is scaffolding, not a stable public command contract.

If the experiment succeeds, the intended provider-neutral product loop is:

```text
tacita propose [--profile <id> ...] --output tacita.proposed.yml <repository>
tacita ratify --proposals tacita.proposed.yml --manifest tacita.yml
tacita check --base <revision> --head <revision> <repository>
```

No candidate is enforceable before explicit ratification. New rules begin at
`warn`; promotion to `block` is per rule and explicit.

The proposals file is never enforced. Ratification presents one candidate at a
time with `accept`, `reject`, and `defer`, and writes the manifest only after
final confirmation. Updates are atomic, preserve unrelated manual rules, and
reject identifier or concurrent-edit conflicts. Checking reads only the
ratified manifest and evaluates only the explicit base/head diff.

Provider-specific CI wrappers may present results later, but the core command
does not infer CI environment variables, call forge APIs, or access the
network.

## Package boundaries

Create a package only when its responsibility becomes independently testable:

```text
cmd/tacita      process boundary and dependency wiring
internal/gitlog Git execution, parsing, normalization, diagnostics
internal/mining pure aggregation and descriptive metrics
internal/backtest temporal splits, baselines, experiment gates
internal/report versioned model and deterministic JSON/text rendering
```

Keep packages one level deep. `internal` is a compiler-enforced boundary, not a
junk drawer. Do not add `pkg`, `utils`, `helpers`, generic repository layers, or
interfaces owned by implementations.

Prefer standard interfaces such as `io.Reader` and `io.Writer`. Define a
consumer-side interface only after real substitution appears; return concrete
types.

## Git process boundary

The analyzed repository is untrusted input. Git invocation must:

- use `exec.CommandContext` with an argument array, never a shell;
- pass paths and revisions as separate arguments;
- resolve a revision first with:

  ```text
  git -C <repository> rev-parse --verify --end-of-options <revision>^{commit}
  ```

- validate the complete object ID and use only that ID in later history
  commands;
- request NUL-delimited machine output;
- cap stdout, stderr, elapsed time, commits, paths, components, pairs, and
  report size;
- set deterministic locale/timezone and neutralize ambient Git configuration,
  paging, optional locks, external diff, and text conversion;
- avoid hooks, filters, difftools, user-defined commands, repository mutation,
  and network access;
- wait for every child process on cancellation.

A `--` path separator alone does not make an untrusted revision safe.

## History ingestion

The initial experiment reads non-merge commits reachable from an explicitly
resolved revision. Ordering uses committer timestamps plus object-ID
tie-breaking. Reports identify the resolved commit, included timestamp range,
explicit `as_of` time, Git/Tacita versions, parameters, and shallow status.

Shallow history is rejected unless a separately approved research mode marks
the report incomplete.

Normalization must:

- deduplicate paths;
- preserve additions, modifications, deletions, and renames;
- apply explicit path and commit-size exclusions;
- retain a count for every exclusion reason;
- avoid claiming rename continuity without a tested identity model;
- read historical generated-file evidence from bounded Git blobs when path
  heuristics are insufficient.

Path bytes may contain spaces, tabs, newlines, leading dashes, and non-UTF-8
data. Parser boundaries must not depend on line-oriented text.

## Resource limits

Milestone 0 freezes numeric values for:

- commits scanned;
- paths per commit and unique paths;
- components per commit and unique components;
- directional component pairs;
- Git stdout and stderr bytes;
- elapsed time;
- report rows and output bytes.

Budget exhaustion is a typed failure with diagnostics. It is never silent
truncation or an empty successful report.

## Reports

JSON is the machine contract. Text renders from the same typed model.

For identical resolved inputs and configuration, canonical output must be
byte-identical. Therefore:

- sort all map-derived output;
- use stable final tie-breakers;
- define floating-point formatting;
- exclude wall-clock, process-local, and temporary-path values;
- place runtime and peak-memory observations in a separate operational sidecar.

Profile and inferred candidates preserve distinct provenance and evidence in
every representation.

## Errors and diagnostics

- Wrap errors with operation context and `%w`.
- Branch on error identity, never message text.
- Distinguish usage, incomplete history, budget exhaustion, Git failure,
  cancellation, and internal invariant failure.
- Do not expose unbounded arbitrary Git stderr.
- Do not log and return the same error below the CLI boundary.
- Pass a logger only if structured diagnostics become necessary; reports are
  not logs.

Malformed input, cancellation, and exhausted budgets must never become
success-shaped empty results.

## Security boundary

Assets at risk include CPU, memory, disk, deterministic output, and the user's
local environment. Repository paths, metadata, blobs, object counts,
configuration, and history shape may be adversarial.

Controls are argument-array execution, NUL framing, bounded work, cancellation,
configuration isolation, read-only object access, no network, minimal
dependencies, vulnerability scanning, race tests, fuzzing, and deterministic
output tests.

Tacita is a local process with the user's privileges. It is not a sandbox, and
hooks are not a security boundary.

## Testing

Use table-driven unit tests for:

- transaction eligibility and exclusion reasons;
- path-to-component projection;
- weighting formulas and zero denominators;
- stable ordering and unusual path bytes;
- temporal leakage prevention;
- report schema/rendering;
- error identity and cancellation.

At the Git boundary, use the real installed Git executable with repositories
under `t.TempDir()`. Do not mock Git. Integration cases include unusual
filenames, option-like revisions, all change types, merges, shallow history,
oversized commits, equal timestamps, cancellation, and repeated byte identity.

Fuzz the NUL parser, path/status decoding, normalization, projection, and report
decoding with deterministic seed cases.

The repository gates are:

```text
make check
make quality-gate
```

The full gate includes formatting, vet, lint, race tests, coverage, static
build, and reachable-vulnerability analysis.

## Concurrency

Begin sequentially. Before adding a goroutine, record:

- starter and lifetime owner;
- channel ownership and closure;
- cancellation path;
- bound on fan-out;
- send completion;
- error propagation;
- awaited shutdown;
- deterministic result ordering.

Benchmark first. Independent repositories or cutoffs are a more plausible
bounded-concurrency boundary than concurrent mutation of one candidate map.

## Distribution

The first deliverable is a static native Tacita binary built with
`CGO_ENABLED=0` and `-trimpath`. Tacita still requires an installed Git
executable at runtime.

A `scratch` image cannot run this design because it lacks Git. Do not replace
Git with a larger pure-Go implementation for packaging optics. OCI packaging
and release automation require separate product evidence and approval.
