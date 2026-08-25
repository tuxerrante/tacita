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

Inside `internal/gitlog`, a concrete run-scoped repository value carries the
classified repository target, the validated environment, and the preflight
result, and exposes resolution and traversal as methods. It exists to make the
required ordering unrepresentable — no object access before containment and
preflight — not to cache work; one run resolves one tip, so avoiding a repeated
version check would not justify it. Its lifetime is one analysis run. It is a
concrete type, not an interface, and it is not a reusable long-lived handle.

Go cannot prevent a caller from writing the zero value of an exported struct, so
the invariant is enforced by failing closed rather than by construction alone.
Every Git invocation passes through one binding step that rejects a repository
the constructor did not produce. That guard is not defensive noise: an
unvalidated value carries no target, and Git without an explicit target falls
back to discovery from the process working directory, which is the containment
failure [repository targeting](#repository-targeting) removes.

For the same reason the constructor resolves the supplied path to an absolute
path first, before any subprocess runs. A relative path names a different
directory after the process changes working directory, so a run could otherwise
continue against a repository that never passed classification, object-format
validation, or preflight. The path is not canonicalized further: symlinks stay
kernel-resolved, as they are for the target arguments.

The constructor validates containment, platform, Git version, and object
format. It does not validate repository completeness; missing objects are
classified during traversal.

## Git process boundary

The analyzed repository is untrusted input. Git invocation must:

- use `exec.CommandContext` with an argument array, never a shell;
- pass paths and revisions as separate arguments;
- target the repository explicitly instead of relying on Git's discovery walk;
- resolve a revision first with:

  ```text
  git <repository-target> rev-parse --verify --end-of-options <revision>^{commit}
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

### Repository targeting

`git -C <path>` does not confine Git to `<path>`. Git walks upward until it
finds a repository, so a path that is not a repository silently resolves
against an ancestor repository and analyzes a target the operator never named.
That is target confusion and an unintended read of unrelated local history, not
a determinism defect: the resolved object ID still identifies the analyzed
history.

`GIT_CEILING_DIRECTORIES` and `GIT_DISCOVERY_ACROSS_FILESYSTEM` are not
sufficient controls. The first is a colon-separated list and therefore cannot
express a path containing `:`; the second only stops mount crossings.

Tacita therefore removes discovery instead of detecting it afterwards. It
classifies the supplied path once, with a filesystem check and no Git process,
and then fixes the target arguments for the whole run:

| Supplied path | Classification | Target arguments |
| --- | --- | --- |
| contains a `.git` directory or gitfile | worktree | `--git-dir=<path>/.git --work-tree=<path>` |
| contains `HEAD` | bare | `--git-dir=<path>` |
| neither | rejected | none |

A gitfile covers linked worktrees and submodule worktrees, so both keep working
without a separate case. A symlinked repository path is accepted and resolved
by the kernel. A path that is neither shape fails with a typed error before any
revision is dereferenced.

`--work-tree` is set only for the worktree shape; the frozen history commands
read objects and never touch the working tree.

### Implemented scalar Git boundary

`internal/gitlog.Open` implements the first Milestone 1 boundary. It validates
the environment, classifies the repository target, runs the completeness
preflight, and returns the run-scoped value whose `ResolveCommit` method
performs the frozen resolution. Together they:

- reject empty inputs, unsupported platforms, Git older than 2.43, and
  non-SHA-1 repositories;
- bind every repository command to an explicit Git directory, so a path that
  is neither a worktree nor a bare repository is rejected instead of resolving
  an ancestor repository;
- refuse to run Git at all through a repository value the constructor did not
  produce, and bind the repository path before any later working-directory
  change can redirect it;
- reject shallow repositories, non-empty grafts and object alternates, and
  promisor or partial-clone configuration before any history command can run;
- execute `git version`, `rev-parse --show-object-format`, the preflight
  queries, and the frozen resolution command with a fixed environment
  allowlist;
- apply the required repository-local configuration overrides;
- capture at most 4 KiB from each fixed-grammar stdout, 1 MiB from the
  effective configuration listing, and 1 MiB from Git stderr;
- stop the child at a stream limit instead of paying for the whole overflow;
- return a complete lowercase 40-byte commit ID or a classifiable error;
- kill and wait for Git when the caller's context is cancelled.

The caller owns the elapsed-time deadline. Successful resolution establishes an
immutable commit identity but does not establish complete history. Preflight
removes the known incompleteness mechanisms. A failed first-parent traversal
uses bounded, machine-readable diagnostics to classify proven unavailable
objects; failures without that evidence remain conservative Git failures.

`FirstParentEvents` implements the first streamed boundary. It runs the frozen
`rev-list` command and returns one event per first-parent commit, root first,
carrying each event's object ID and whether it is the root, an ordinary commit,
or a merge result.

Its parser reads one object ID at a time and never holds a line, so an octopus
merge that prints every parent on one line costs the same as a root line. It
validates the chain while reading it: the first event is the chain's root, and
every later event names its predecessor as its first parent, so a stream that
skipped or reordered an event fails instead of producing a chain that never
existed. Events are retained as 41-byte values rather than strings, which keeps
the frozen 200,000-event cap under 8 MiB.

The `rev-list` command carries no `--end-of-options`, so its revision argument
is validated as a complete lowercase object ID before the command runs.

Total streamed bytes are bounded by the execution layer rather than the parser,
which bounds only memory. Reaching the byte budget stops the child, and the
resulting truncation is reported as an output limit rather than as the killed
process or the malformed stream it also produces.

This boundary is provisional and under review.

A bounded writer fails the write once its limit is reached rather than
discarding the remainder. `os/exec` then closes its end of the pipe, and the
writer cancels a context Tacita owns, which kills the child. Retained bytes are
bounded by the limit, and the child is torn down on the first copied chunk that
crosses it, so the overflow costs one copy buffer rather than the whole stream:
measured against a 400 MB blob, the discarding writer read all of it in 972 ms,
and the failing writer stopped after one 32 KiB chunk in 4 ms.

The caller's context is left untouched, so a run stopped by a limit is still
classified as an output-limit failure and not as a caller cancellation or a
killed process.

`EachEventChange` implements the second streamed boundary. It runs the frozen
`diff-tree` command, feeds it the event chain `FirstParentEvents` produced, and
hands each event's normalized paths to a visitor as they are decoded. Nothing is
accumulated across events: 200,000 events holding 2,000 paths each would exceed
the resource budget, so the caller decides what to keep. Each visited slice is
uniquely owned by the receiver and is never reused.

The decoder dispatches on a single byte. A record always begins with `:`, and an
object ID never does, so a boundary needs no lookahead. Every boundary is
compared against the next expected event, which makes a desynchronized stream a
failure rather than evidence attributed to the wrong commit.

Exclusion precedence is applied per event in a fixed order, so counts stay
disjoint and a repository cannot spend another repository's budget:

1. raw paths are deduplicated first;
2. the 2,000-path event limit is applied to distinct raw paths *before* vendor
   and gitlink filtering, so unlimited excluded paths cannot evade the budget;
3. event reasons are `root_event`, then `event_path_limit`, then
   `no_eligible_paths`;
4. an event over the path limit contributes only `event_path_limit` and
   discards its provisional path counts;
5. a vendored gitlink counts only as `vendor_path`.

Steps 2, 4, and 5 resolve a genuine gap: the frozen text names the reasons but
never fixed their precedence. They are ratified here and are the behavior the
implementation pins.

Retained memory per event is bounded by the path limit, but a single path is
bounded only by the 1 GiB stream budget. No separate path-length limit was
invented, so the strict per-event memory claim rests on the output cap rather
than on a path bound.

This boundary is provisional and under review.

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

The frozen reference environment uses Git 2.55.0 and Go 1.27.0. Calibration,
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
directory. Reject a graft or alternate entry whose path is not a regular file
rather than reading it, so a symlink, FIFO, or device cannot redirect or block
the run, and bound every such read. Reject a file that exceeds that bound
instead of judging it by the prefix that was read. Read promisor configuration
only to reject the repository, from the effective local and worktree scopes
rather than the local scope alone, because `includeIf` and worktree-scoped
configuration otherwise hide it. Git normalizes configuration keys to lowercase,
so match `extensions.partialclone`, `remote.<name>.promisor`, and
`remote.<name>.partialclonefilter` case-insensitively. After a failed
first-parent commit traversal, Tacita runs the same walk with bounded,
machine-readable `--missing=print` diagnostics. A successful diagnostic
containing only `?<full-object-id>` records proves an unavailable object and
is `incomplete_repository`. Diagnostic failure, overflow, or malformed output
preserves the original Git failure; stderr text is never a branching input.
The bounded run does not perform a full `git fsck`.

The effective configuration is obtained with one bounded `config --list -z`
rather than a scope-restricted listing. `--local` does not expand a conditional
include, and `--worktree` fails outright once a linked worktree exists without
the worktree-config extension, so either would let a repository hide the keys
being searched for. System and global configuration are already disabled through
the child environment, which leaves the plain listing equal to the effective
repository scopes. A key listed without a value is boolean true, which is how a
promisor remote appears when written in its shorthand form. A partial-clone
filter is rejected on presence alone, because Git registers a promisor remote
for it whatever the filter says, without requiring any promisor key.

Preflight rejects known incompleteness mechanisms; it cannot prove that every
required object is present. The frozen data flow omits `--root`, so the root
commit's tree is never read, and objects reachable only through secondary
parents are never traversed. Complete local objects therefore remain a support
precondition, and missing objects discovered during traversal are classified,
not prevented. Because Git offers no stable machine-readable distinction
between a missing object and other failures for every history command, and
stderr text is not a branching input, any unclassifiable traversal failure is
reported conservatively rather than guessed. In particular, `diff-tree` does
not expose a causal machine-readable missing-tree diagnostic. A broad object
walk would inspect objects the frozen diff never needs and could misclassify an
unrelated failure, so it is not used.

Rejecting promisor and partial-clone repositories is a correctness *and*
isolation requirement. `diff-tree` over a partial clone lazily fetches the
missing objects it needs: it reaches the network and writes new packs into the
repository, which breaks both the offline and the read-only guarantee. This
happens during traversal, not during revision resolution, so the check must
complete before the resolved ID reaches the history commands.

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

The success path uses two bounded Git commands with the frozen environment,
repository target, and `-c` overrides:

```text
git <repository-target> rev-list \
  --first-parent --reverse --parents <resolved-object-id>

git <repository-target> diff-tree \
  --stdin --always -r --raw -z --abbrev=40 \
  --no-renames --no-textconv --no-ext-diff \
  --diff-merges=first-parent
```

If the first command exits unsuccessfully, Tacita may run one bounded
diagnostic over the same first-parent walk:

```text
git <repository-target> rev-list \
  --quiet --first-parent --missing=print <resolved-object-id>
```

This failure-only command does not contribute events or paths. Its output is
used solely for the conservative incomplete-repository classification described
in [supported experiment environment](#supported-experiment-environment).

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
- match every boundary against the expected `rev-list` object-ID sequence, so a
  desynchronized stream fails instead of misattributing paths to an event;
- exclude the root event as `root` even when it also has no eligible paths, so
  exclusion reasons stay disjoint and countable;
- avoid reading file contents or historical blobs during the first experiment.

Path bytes may contain spaces, tabs, newlines, leading dashes, and non-UTF-8
data. Parser boundaries must not depend on line-oriented text.

### Bounded streaming contract

Neither command's output may be buffered whole.

`--first-parent` restricts which commits are listed, not how many parents each
listed commit prints. An octopus merge on the chain prints all of its parents
on one line, so a `rev-list` line has no fixed upper length and the command's
output can approach the 1 GiB stdout cap while the run must stay inside a 2 GiB
resident budget. Parse `rev-list` incrementally, validate every field as a full
object ID, and retain only the first-parent metadata the run needs. Retaining
the first object ID of each event is bounded to roughly 8 MiB at the 200,000
event limit, which is what `diff-tree` receives on stdin. Do not configure a
token-oriented reader with a near-1-GiB allowance.

`diff-tree` stdout must be streamed for the same reason and decoded as NUL
records, never as lines.

Execution stays sequential and Tacita owns no goroutine. Supplying `diff-tree`
stdin as an in-memory reader makes `os/exec` own the finite stdin-writing
goroutine, while Tacita reads stdout itself. This is deadlock-free only on the
success path: waiting on the child before stdout reaches EOF can block Git
forever on a full pipe, and `WaitDelay` does not start until the wait begins.
Therefore, when a budget or grammar failure stops parsing early, cancel the
child's context, drain stdout to EOF, wait, and only then return the saved
failure. The original context's failure, if any, wins over the parser's.

The drain runs whether or not parsing failed, since a parser that stopped early
without failing blocks Git just as effectively. Output the parser never read
still counts against the byte budget, so the drain runs through the bounded
reader before it reaches EOF on the raw pipe; otherwise a parser that succeeded
early would let a repository produce unbounded output for free.

A per-event exclusion is not a stop condition. Excluded events are consumed and
counted, and parsing continues; only global exhaustion and malformed input end
the stream.

The bounded scalar writer used for fixed-grammar output is not reusable for
this stream.

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

The transaction stream owns the repository-wide distinct-path and
distinct-component budgets. It counts identities only from eligible
transactions, after per-event exclusions and before invoking the visitor.
Repeated identities across events count once. Path identities are checked
before component identities, and the first observed identity over either limit
fails the run with its observed count and limit. No transaction that crosses a
global identity limit reaches the visitor.

## Aggregation strategy

The [experiment protocol](experiment.md) owns the metric formulas. This section
owns only how they are computed.

Read naively, the protocol asks for 81 configurations evaluated at 4 cutoffs,
which reads like 324 traversals of history and invites parallelism. It is not.
The frozen definitions collapse into a single accumulation:

- the cutoffs are expanding, and training membership is nested, so a later
  cutoff's totals are an earlier cutoff's totals plus the events in between;
- the first experiment applies no temporal decay and a retained transaction
  keeps its weight as the cutoff advances, so those totals are running sums;
- of the four grid parameters, only the size weight changes what is
  accumulated. The minimum opportunity, confidence, and lift thresholds are
  post-aggregation filters over already-accumulated values, so 3 accumulation
  states cover all 81 configurations.

Therefore one ordered pass maintains three weighted sums side by side and
snapshots at each cutoff boundary. Cutoff boundaries are defined over all
integration events before eligibility exclusions, so `rev-list` already fixes
`N` and the four boundary object IDs before `diff-tree` streams any change.

Aggregate state is split by what each value is keyed on, because the formulas
are not all per-pair:

| State | Key | Holds | Bound |
| --- | --- | --- | ---: |
| Component | component identity | opportunity, exposure, prevalence numerator | 50,000 |
| Pair | ordered component pair | support | 2,000,000 |

Keying opportunity and exposure on the pair would be wrong as well as slow: a
transaction containing the antecedent updates that antecedent's opportunity
even when the consequent is absent, so a pair-keyed representation would have
to touch every pair the antecedent has ever appeared in.

Intern component identities to integer indices for the hot keys and restore the
canonical component bytes only when rendering, so the pair key stays a small
pointer-free value and ordering stays defined by the report contract.

The chronological fold cannot produce the hash-shuffled control, which
deliberately reorders events and destroys chronology. Normalized events are
therefore retained in memory and the same accumulation runs again over the
control ordering. Keeping accumulation a pure function of an event sequence is
what makes the control almost free; it is also the clearest form.

This strategy depends on the frozen no-decay decision. If a later experiment
introduces temporal decay, retained weights change as the cutoff advances, the
running-total fold is no longer valid, and this section must be revisited
before the code is.

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

A fuzz target asserts a property, never merely the absence of a panic. Each one
carries a seed for every rule it covers, so `go test` alone fails when a rule is
removed and no mutation has to invent Git's exact keys. Where the decision worth
fuzzing sits behind a subprocess or a file, the decision is a separate function
over bytes or an `io.Reader`, while opening and running stay where they are.

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

Benchmark first.

Cutoffs and grid configurations are no longer candidate concurrency boundaries.
They look independent, but the [aggregation strategy](#aggregation-strategy)
shows they fold into one pass, and folding beats parallelism here on every
axis: it removes work rather than redistributing it, it keeps one shared map
instead of duplicating state against the 2 GiB budget, it cannot perturb the
frozen byte-identical output, and it is less code. A goroutine per cutoff would
buy at most the runner's four cores while giving up all of that.

Independent repositories remain genuinely parallel, and they need no
concurrency inside Tacita: one analysis run targets one repository, so the
external harness parallelizes by running the binary once per repository. This
keeps process isolation, per-run memory accounting, and per-run failure
classification intact.

That leaves no in-process fan-out in the first experiment. Revisit only if a
sequential benchmark shows a real bottleneck.

## Distribution

The first deliverable is a static native Tacita binary built with
`CGO_ENABLED=0` and `-trimpath`. Tacita still requires an installed Git
executable at runtime.

A `scratch` image cannot run this design because it lacks Git. Do not replace
Git with a larger pure-Go implementation for packaging optics. OCI packaging
and release automation require separate product evidence and approval.
