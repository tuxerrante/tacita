# Architecture

This is a technical reference for implementing Tacita safely. It assumes the
product vocabulary and workflow from the [product brief](product.md). Read that
first if `candidate`, `profile`, `ratification`, or `finding` is unfamiliar.

The [experiment protocol](experiment.md) owns mining formulas, human-review
labels, samples, and acceptance gates. This document owns CLI, Git, package,
resource, report, security, and testing contracts. See the
[documentation map](README.md) for task-specific reading paths.

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

The [product brief](product.md#product-loop) owns the future workflow,
ratification semantics, and enforcement policy. If the experiment succeeds,
the provisional provider-neutral command shape is:

```text
tacita propose [--profile <id> ...] --output tacita.proposed.yml <repository>
tacita ratify --proposals tacita.proposed.yml --manifest tacita.yml
tacita check --base <revision> --head <revision> <repository>
```

The command contract is not frozen. Its technical constraints are that
proposal output is never enforceable, manifest updates are atomic, unrelated
manual rules are preserved, identifier and concurrent-edit conflicts are
rejected, and checking reads only the ratified manifest and explicit base/head
diff.

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
- cap stdout, stderr, elapsed time, integration events, paths, components,
  pairs, and report size;
- set deterministic locale/timezone and neutralize ambient Git configuration,
  paging, optional locks, external diff, and text conversion;
- avoid hooks, filters, difftools, user-defined commands, repository mutation,
  and network access;
- wait for every child process on cancellation.

A `--` path separator alone does not make an untrusted revision safe.

## Supported experiment environment

The first experiment supports:

- Linux on `amd64`;
- Git 2.43 or newer;
- repositories using Git's SHA-1 object format;
- bare repositories and worktrees available through a local filesystem path.

Other operating systems, architectures, older Git versions, SHA-256
repositories, object alternates, and promisor repositories are unsupported.
Detect these conditions before history traversal and return a typed
unsupported-environment or incomplete-repository error.

The frozen reference environment uses Git 2.55.0 and Go 1.26.5. Calibration,
holdout, and repeated determinism runs use the exact same Git version, Tacita
binary digest, and optional Go helper version recorded in the development lock.

Resolve the object format with `rev-parse --show-object-format`. A resolved
commit ID must be exactly 40 lowercase hexadecimal bytes. No abbreviated or
symbolic revision may cross the resolution boundary.

Invoke Git with `LC_ALL=C`, `TZ=UTC`, `GIT_OPTIONAL_LOCKS=0`,
`GIT_NO_REPLACE_OBJECTS=1`, system and global configuration disabled, an empty
pager, and command options that disable external diff, text conversion, and
rename detection. Repository-local hooks are never invoked. Failure caused by
repository ownership or safety checks is reported; Tacita never modifies
`safe.directory`.

Reject repositories with a non-empty legacy `info/grafts` file. Replacement
refs remain disabled rather than interpreted. Reject object alternates,
promisor remotes, and missing local objects.

Override relevant repository-local configuration with `-c` arguments:
`core.abbrev=40`, `core.fsmonitor=false`, `core.hooksPath=/dev/null`, and
`diff.external=`. Explicit command flags remain authoritative for root,
external-diff, text-conversion, merge-diff, and rename behavior.

Preflight uses `rev-parse --is-shallow-repository` and
`rev-parse --path-format=absolute --git-common-dir`. Inspect graft and alternate
files relative to the common directory, not a linked worktree's private Git
directory. Read local `extensions.partialClone` and `remote.*.promisor`
configuration only to reject the repository. Any missing-object failure during
traversal is `incomplete_repository`; the bounded run does not perform a full
`git fsck`.

Build the Git child environment from an allowlist. Clear inherited repository,
object-store, index, worktree, tracing, prompt, and attribute-source variables,
including `GIT_DIR`, `GIT_WORK_TREE`, `GIT_COMMON_DIR`,
`GIT_OBJECT_DIRECTORY`, `GIT_ALTERNATE_OBJECT_DIRECTORIES`, `GIT_INDEX_FILE`,
`GIT_ATTR_SOURCE`, `GIT_TRACE*`, and credential or terminal prompt variables.

## Integration history

The initial experiment models changes as integration events on the first-parent
chain of the explicitly resolved commit:

- traverse the first-parent chain from its root to the resolved commit;
- each chain commit defines one integration event;
- for a single-parent commit, diff it against its parent;
- for a merge commit, diff it against its first parent so the event represents
  the net change introduced onto the integration line;
- record the root commit but exclude its empty-tree diff from mining.

This includes merge results as transactions but does not separately mine the
internal commits of merged branches. A merge's other parents remain provenance
and are not independent transactions. Octopus merges follow the same
first-parent rule.

A squash merge appears in Git as one new single-parent commit on the integration
line. Its net file change is therefore preserved as one transaction, but local
Git data cannot distinguish it from an ordinary direct commit.

Rebase and fast-forward integration preserve multiple single-parent commits on
the integration line. Without forge metadata, Git does not preserve the pull
request boundary needed to combine them into one transaction. The
provider-neutral experiment therefore treats each resulting commit as a
separate integration event and reports this merge-policy sensitivity as a
limitation. It must not infer review boundaries from commit messages or call a
forge API.

Merge commits can have the opposite sensitivity: unrelated concerns combined
in one merge become one large transaction. Squash integration tends to match
the desired one-review-unit-per-transaction model, while rebase, fast-forward,
and broad merge commits can respectively fragment or bundle review units.

First-parent ancestry, not commit timestamps, defines event order and
availability. At a cutoff commit, only that commit and its first-parent
ancestors are available. Clock skew cannot reorder events.

An **eligible integration event** is a chain event that remains after all
frozen path, size, history, and resource exclusions. Each eligible event
becomes one transaction; every exclusion is counted by reason.

The root event, events with no eligible paths, and events exceeding the frozen
path or component limits are excluded with distinct reasons. Root exclusion
prevents an initial repository snapshot or template import from fabricating
co-change among every initial component.

Reports identify the resolved commit, first-parent root and event counts,
Git/Tacita versions, and parameters.

Shallow history is rejected in the first experiment.

### Frozen Git data flow

Use two bounded Git commands with the frozen environment and `-c` overrides:

```text
git -C <repository> rev-list \
  --first-parent --reverse --parents <resolved-object-id>

git -C <repository> diff-tree \
  --stdin --always -r --raw -z --abbrev=40 \
  --no-renames --no-textconv --no-ext-diff \
  --diff-merges=first-parent
```

The first command is line-oriented only because every field is a validated
full hexadecimal object ID. It defines event order, first parent, and event
kind. Tacita feeds only its first object-ID field from each line to the second
command's stdin.

The second command emits one `<commit-object-id>\0` boundary for every input,
including empty events. Each following raw record is:

```text
:<source-mode> <destination-mode> <source-oid> <destination-oid> <status>\0
<path>\0
```

Modes are six octal bytes; object IDs are 40 lowercase hexadecimal bytes.
Accepted status bytes are `A`, `D`, `M`, and `T`. `R` or `C` violates the
no-rename invariant; every other status is malformed input. Mode `160000`
identifies a gitlink. A commit boundary with no following raw record is an
event with zero changed paths.

Normalization must:

- deduplicate paths;
- preserve additions, modifications, deletions, and type changes;
- disable rename and copy detection, representing a move as deletion plus
  addition;
- apply explicit path and integration-event-size exclusions;
- retain a count for every exclusion reason;
- record whether evidence came from a single-parent, merge-result, or root
  event;
- avoid reading file contents or historical blobs during the first experiment.

Path bytes may contain spaces, tabs, newlines, leading dashes, and non-UTF-8
data. Parser boundaries must not depend on line-oriented text.

## Component projection

Projection is lexical and repository-relative:

1. normalize Git's `/` separator without filesystem access, Unicode
   normalization, case folding, symlink resolution, or path cleaning;
2. exclude a path if any segment is exactly `vendor`;
3. exclude gitlink entries representing submodules;
4. map every remaining path to its parent directory;
5. map a root-level file to the component `.`.

Nested Go modules do not create implicit component boundaries. A directory
containing Go files may receive descriptive package metadata, but its component
identity remains its repository-relative directory bytes. Deleted-only
directories remain valid historical component identities. Symlinks are treated
as path evidence and their targets are never read or followed.

With rename detection disabled, a cross-directory move contributes both the
old and new parent components to the same integration event. A same-directory
move contributes that component once after deduplication.

Generated files, lockfiles, fixtures, documentation, and non-Go languages are
included unless they match a frozen path exclusion above. The first experiment
does not infer generated status from names or contents.

## Resource limits

The first experiment uses these hard limits per repository run. A calibration
run evaluates all 81 configurations in one process and one analytical report:

| Resource | Limit | Exhaustion behavior |
| --- | ---: | --- |
| First-parent integration events scanned | 200,000 | fail |
| Changed paths in one non-root event | 2,000 | exclude event |
| Distinct path identities | 2,000,000 | fail |
| Components in one event | 100 | exclude event |
| Distinct component identities | 50,000 | fail |
| Directional pair observations | 20,000,000 | fail |
| Distinct directional pairs | 2,000,000 | fail |
| Captured Git stdout | 1 GiB | fail |
| Captured Git stderr per command | 1 MiB | fail |
| Analysis elapsed time | 20 minutes | cancel and fail |
| Peak resident memory | 2 GiB | terminate and fail |
| Canonical report rows | 100,000 | fail |
| Canonical report bytes | 64 MiB | fail |
| Operational sidecar bytes | 1 MiB | fail |

Root events and events with no eligible paths are named exclusions independent
of these limits. Per-event path and component limits exclude the complete event
before aggregation; they never retain a prefix. Every global exhaustion is a
typed failure with the observed count and limit. It is never silent truncation
or an empty successful report.

Runtime and memory acceptance use a Linux `amd64` reference runner with four
logical CPUs, 8 GiB RAM, a local complete clone, no network access, and
sequential Tacita execution. Clone time is excluded. The external experiment
harness records monotonic elapsed time and peak resident set size in the
operational sidecar; those observations do not enter canonical report bytes.

Tacita enforces elapsed time with context cancellation and emits
`budget_elapsed` when it can report the breach. Peak resident memory is
enforced by the external harness. A harness kill is recorded through sidecar
exit status; canonical `failure.observed` remains `null` for elapsed or memory
limits not observed by Tacita itself.

## Reports

The frozen machine contract is [`report-v1.md`](report-v1.md). JSON is
authoritative; text renders from the same decoded typed model.

For identical resolved inputs, configuration, Tacita binary, and Git version,
canonical output must be byte-identical. Therefore:

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

The detailed assets, trust boundaries, threats, controls, and exclusions are in
the frozen [`threat-model.md`](threat-model.md).

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
filenames, option-like revisions, all change types, merge-result diffs,
squash-shaped single-parent histories, rebase/fast-forward histories, shallow
history, oversized events, equal timestamps, cancellation, and repeated byte
identity.

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
