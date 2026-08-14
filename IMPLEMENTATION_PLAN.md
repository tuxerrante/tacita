# Tacita Implementation Plan

Status: Milestone 0 specification plus a disposable CLI learning bootstrap;
product implementation remains blocked
Source brief: [`PRODUCT_BRIEF.md`](PRODUCT_BRIEF.md)
Initial product milestone: evidence-first vertical slice

## 1. Purpose

Tacita asks one product question:

> Does this change deviate from a repository-local decision reflected in how
> this repository has historically changed?

Its initial user is a maintainer of a Go repository reviewing an explicit diff,
locally or in CI. Mature repositories bootstrap contracts from history; new
repositories bootstrap them from independently selected profile tiers.

Git history cannot answer that question by itself. It can only surface
candidate expectations and their exceptions. A human decides whether a
candidate reflects project intent or an accident; only a ratified expectation
can later become a check.

The first job is therefore not enforcement and not next-change recommendation.
It is to prove, with reproducible evidence, that history can produce a small set
of stable, explainable candidates that maintainers judge worth ratifying.
Holdout co-change behavior is supporting evidence for those candidates, not the
product claim.

The first vertical slice is:

```text
Git history
  -> normalized transactions
  -> candidate repository expectations
  -> evidence and historical exceptions
  -> temporal and product-relevance evaluation
  -> deterministic proposal reports
```

If that experiment fails its predeclared gates, development stops or the product
hypothesis changes. Thresholds must not be lowered merely to produce output.

The eventual product loop, if the experiment succeeds, is:

```text
discover from history or explicit profile -> human ratifies intent
  -> versioned repository contract
  -> tacita check evaluates an explicit diff
  -> changed diff deviates from contract -> explainable review finding
```

An unratified candidate is never a violation. Tacita does not recommend future
features or claim that every historical regularity is architectural intent.
Profile candidates are curated guidance, not evidence inferred from the
repository. Their origin must remain visible at every step.

## 2. Corrections to the historical brief

`PRODUCT_BRIEF.md` defines the product. This document defines the implementation
experiment and supersedes earlier local drafts where they conflict.

### 2.1 Weighted metrics are descriptive, not inferential

The historical brief combines fractional commit-size and age weights with
Wilson intervals and Fisher's exact test. Wilson and Fisher assume integer
Bernoulli/count data; they cannot justify confidence intervals, p-values, or
false-discovery-rate claims over these fractional weighted sums.

The first implementation may report:

- raw transaction counts;
- weighted support;
- weighted exposure;
- weighted confidence;
- weighted lift;
- temporal stability and holdout behavior.

It must not report:

- Wilson bounds over weighted observations;
- Fisher p-values over fractional contingency tables;
- Benjamini-Hochberg q-values derived from those p-values;
- statistical significance or expected-FDR claims.

A future inference phase would need its own reviewed method, likely a weighted
permutation procedure, and a performance/reproducibility justification.

### 2.2 Commit-size weighting is a hypothesis

The proposed weight `1 / (|components|-1)` reduces some large-commit influence
but does not normalize each commit's total pair contribution. It is also
undefined for single-component commits.

The backtest must compare at least:

1. unit weight after the hard commit-size filter;
2. inverse size: `1 / max(1, |components|-1)`;
3. pair-normalized weight:
   `1 / max(1, |components| * (|components|-1))` for directed pairs.

The hard ingestion filter still uses changed-path count because a bulk commit
can be operationally unhelpful even when all files project to few components.
Reports retain both path and component counts.

Every report keeps raw counts beside weighted values. No weighting mode becomes
the default until it beats simpler modes on development repositories and an
untouched holdout.

The first score does not multiply by a separate `Heat(A)` term. Exponential
decay already represents recency; adding heat would count it twice.

### 2.3 Backtesting does not establish causality

A later commit that changes A without B is not automatically defective.
Co-change can reflect architecture, but it can also reflect commit habits,
tangled work, generated files, or repository process.

The backtest measures whether a candidate remains a regularity:

- when A changes, how often does B appear in the same future transaction?
- how much better is that result than trivial baselines?
- how many eligible changes exercise a candidate expectation?
- how stable are the highest-ranked rules over adjacent time cutoffs?
- how often does the analysis abstain?

That is necessary but insufficient for the product. The evaluation must also
measure whether blinded reviewers classify candidates as reflecting a project
decision, an accident, or unclear evidence; whether candidates rediscover
predeclared documented repository contracts; and whether sampled holdout
deviations would have merited review. These labels are product evidence, not
proof of causality. Later fix/revert case studies remain qualitative supporting
evidence and must be labelled as such.

### 2.4 Import co-usage is not a layer-boundary detector

Treating each Go package's imports as a transaction can discover packages that
are commonly imported together. It does not, by itself, establish forbidden
dependency direction or architectural layering. Import rules require a
separately specified directed-graph analysis and are outside the first slice.

### 2.5 Container and dependency goals follow runtime reality

The first implementation uses the installed Git executable through
`exec.CommandContext`. A `scratch` image contains no Git and would not run
Tacita. Distribution therefore starts with static native Tacita binaries. If an OCI image
is later justified, its minimal digest-pinned runtime must include a patched Git
binary.

The project must not adopt a pure-Go Git implementation merely to claim a
scratch image. That would enlarge the dependency and parser attack surface for
packaging optics.

### 2.6 Profiles are a separate cold-start source

New repositories cannot produce history-derived candidates. The first product
therefore includes two separately selectable profile tiers as a second proposal
source:

1. `go-core`: a small high-confidence set whose applicability and behavior are
   supported by official Go or toolchain contracts;
2. `spf13-idiomatic`: explicitly opinionated guidance selected from the pinned
   `spf13/go-skills` source.

Both tiers are opt-in. The opinionated tier is not silently enabled by
`go-core`, and no profile is implied merely because the repository contains Go.
Profile rules are selected and versioned explicitly; enabling a profile never
makes its guidance a repository-inferred fact.

An enabled profile proposes every applicable rule, including rules the current
repository already follows. Each proposal reports:

- applicability as `applicable`, `not_applicable`, or `unknown`;
- current state as `compliant`, `deviating`, or `unknown`;
- the repository evidence used for both determinations;
- rationale and exact rule-level provenance.

Only applicable rules enter ratification. Non-applicable and unknown rules
remain visible in diagnostics so silence is explainable. Before ratification,
`deviating` is an observation, not a violation or finding.

Profile and inferred candidates share the same human ratification boundary and
eventual manifest model, but not the same evidence or success metrics. Profile
results never enter co-change ranking, backtest baselines, or the inferred
product gate. A successful profile evaluation cannot compensate for a failed
mining hypothesis.

### 2.7 Initial profile catalog

`go-core` does not mean that every Go repository should adopt each rule. It
means the rule's semantics and deterministic evidence come from official Go or
toolchain contracts rather than one author's preferred style. Adoption remains
a repository decision.

The initial `go-core` rules are:

| Rule ID | Proposed repository contract | Applicability and evidence | Primary source |
| --- | --- | --- | --- |
| `module-topology` | Adding, removing, or moving a module root requires ratification | Any repository containing `go.mod`; capture the sorted repository-relative module roots | [Organizing a Go module](https://go.dev/doc/modules/layout) |
| `go-version-floor` | Raising a module's ratified `go` directive requires ratification | Each module with a `go` directive; compare the proposed and ratified versions | [Go directive](https://go.dev/ref/mod#go-mod-file-go) |
| `release-no-local-replace` | A module intended for publication must not retain filesystem-local `replace` targets | Applicability requires the maintainer to mark the module publishable; inspect parsed `go.mod` replacements and their targets | [Replace directive](https://go.dev/ref/mod#go-mod-file-replace) |
| `semantic-import-version` | A published v2+ module path must carry the matching `/vN` suffix | Publishable modules at major version 2 or later; compare release major and declared module path | [Major version suffixes](https://go.dev/doc/modules/major-version) |

The initial `spf13-idiomatic` rules are explicitly opinionated and derive from
[`spf13/go-skills`](https://github.com/spf13/go-skills/tree/e67851cfcca008592c7c4965b8220c7cb37e2f1c):

| Rule ID | Proposed repository contract | Applicability and evidence |
| --- | --- | --- |
| `no-root-pkg` | Do not introduce a root `pkg/` layout | Any Go repository; inspect repository-relative directories |
| `domain-over-layers` | Do not organize packages under layer names such as `models`, `services`, `controllers`, `dto`, or `repositories` | Any Go repository; compare package-directory segments with the frozen denylist |
| `max-package-depth` | Keep package directories at no more than three levels below the module root | Any module containing Go packages; calculate path depth from each module root |
| `no-heavy-test-frameworks` | Do not add the selected mock-generation, mock-object, or BDD frameworks | Repositories containing Go tests; inspect module requirements and Go imports against the frozen `gomock`, `mockery`, `testify/mock`, `ginkgo`, and `gomega` identifiers |

Rule IDs and intent are ratified here. Milestone 0 must still freeze parser
semantics, symlink handling, nested-module behavior, exact module/version
applicability inputs, and canonical identifiers for the framework denylist.
Rules such as "internal junk drawer," mandatory table-driven tests, and
`cmd/` business-logic detection are excluded from the first catalog because
their automated applicability or violation semantics are too subjective.

## 3. Decisions

| Concern | Decision |
| --- | --- |
| First milestone | Evidence-first vertical slice |
| CLI framework | Standard library `flag` initially |
| Runtime module dependencies | Zero until evidence justifies one |
| Manifest format | Not part of the first slice |
| Report format | Versioned JSON plus deterministic text |
| Go layout | Shallow domain packages under `internal/`; no `/pkg` |
| Git access | Installed Git executable, no shell |
| Statistical posture | Weighted descriptive ranking only |
| New-repository cold start | Separate opt-in `go-core` and `spf13-idiomatic` profiles |
| Candidate provenance | `inferred` and `profile` remain distinct through ratification |
| First product workflow | Provider-neutral `tacita check` over an explicit diff, local or CI |
| Integration boundary | Real temporary Git repositories |
| Concurrency | Sequential first; add only after profiling |
| Distribution | Static native binary first |
| Enforcement | Deferred until the evidence gate passes |
| First-product enforcement default | Newly ratified rules start at `warn`; `block` is explicit per rule |

## 4. Scope

### 4.1 Included

- read non-merge commits reachable from an explicit revision, initially `HEAD`;
- extract normalized file-change evidence and project it to component
  transactions;
- apply explicit collection windows, cutoffs, exclusions, and resource limits;
- aggregate directional package/directory-pair candidates;
- calculate raw and weighted descriptive measures;
- evaluate candidates using chronological cutoffs;
- compare against trivial baselines;
- expose supporting transactions and historical exceptions for human judgment;
- evaluate frozen `go-core` and `spf13-idiomatic` profiles on new repositories,
  only when explicitly enabled and with rule-level source/version provenance;
- evaluate product relevance against a preregistered review protocol;
- emit schema-versioned JSON and human-readable proposal reports;
- prove deterministic behavior and bounded resource use.

### 4.2 Explicitly deferred

- `tacita.yml`, `tacita.proposed.yml`, YAML parsing, ratification, and enforcement;
- profile expansion beyond the frozen initial profile rules;
- import-graph and AST mining;
- baseline/ratchet behavior;
- provider-specific CI wrappers and review annotations;
- TUI;
- MCP server;
- generated `ARCHITECTURE.md`;
- Kubernetes CRDs, validation, or an operator;
- OCI images;
- cache design;
- release automation.

These are possible later milestones, not commitments. The visible screenshot
roadmap is supplemental input only because both screenshots are cropped and do
not expose the full original critique.

### 4.3 Permanent non-goals

- attributing code to a human or an AI agent;
- process, PID, cgroup, environment, or packaging-format fingerprinting;
- local-hook claims as a security boundary;
- dataflow, taint, symbolic execution, or runtime invariant inference;
- deep learning or a GNN;
- opaque weighted scorecards;
- extracting executable policy from generated prose;
- network or LLM dependencies in the core analysis path.

## 5. Product hypothesis and falsification

### 5.1 Hypothesis

For repositories with sufficient coherent history, Tacita can surface a small set
of stable, sparse, deterministic, and explainable candidate expectations that:

1. behave as real regularities on unseen history rather than frequency
   artifacts; and
2. maintainers judge to reflect repository decisions whose deviations merit
   review.

The second condition is the product hypothesis. Predictive co-change metrics
test only the first condition.

For new repositories without sufficient history, the cold-start hypothesis is
separate: the transparent, independently opt-in profile tiers can produce at
least three candidate expectations that a maintainer judges appropriate to
ratify within the same 10-minute adoption budget. Every candidate must identify
its profile tier and rule-level provenance; profile results do not validate the
inferred hypothesis.

### 5.2 Baselines

Every experiment compares Tacita with:

1. **Frequent consequent:** rank the most frequently changed eligible
   components regardless of antecedent, using the same training window but no
   commit-size or temporal weighting.
2. **Unweighted co-change:** raw confidence and lift after the same eligibility
   filters, training window, and cutoff, with neither temporal nor commit-size
   weighting.
3. **Time-shuffled control:** preserve transaction sizes and component
   frequencies while breaking temporal association. Use a fixed, reported seed
   and stable PRNG algorithm so the control is reproducible.
4. **No-history control:** with profiles disabled, young/template repositories
   should produce no inferred candidates rather than fabricated certainty.
   With a profile explicitly enabled, profile candidates are evaluated and
   reported in a separate lane.

### 5.3 Provisional go/no-go gate

Milestone 0 freezes exact corpus revisions and thresholds before holdout results
are observed. It must freeze two distinct gates.

The **product-relevance gate** requires a preregistered, blinded review protocol:

- before mining, record documented repository contracts that directional
  co-change could plausibly rediscover;
- after mining, show each candidate's supporting changes and exceptions without
  showing its rank or weighting mode;
- classify each candidate as `decision`, `accident`, or `unclear`;
- independently classify a frozen sample of holdout deviations as
  `review-worthy`, `benign`, or `unclear`;
- record decision time, evidence inspected, and time to the first ratified
  candidate so proposal quality cannot hide an expensive review workflow;
- record reviewer identity relative to the repository and disagreements;
- freeze minimum decision precision and review-worthy deviation precision
  before inspecting the untouched holdout.

The ratified adoption gate is evaluated independently by candidate origin:

- on a mature product-evaluation repository, the end-to-end interval from
  starting Tacita to ratifying the third `inferred` `decision` candidate must be
  at most 10 minutes; mining time is included;
- on a new product-evaluation repository with the selected profile explicitly
  enabled, the interval from starting Tacita to ratifying the third `profile`
  `decision` candidate must be at most 10 minutes. A compliant applicable rule
  counts because the purpose is to establish a future repository contract, not
  merely report existing defects.

Each reviewer must already be familiar with the evaluated repository. A
repository without a qualified reviewer may contribute to engineering or
regularity evidence, but it cannot pass or fail an adoption gate. Milestone 0
must freeze the benchmark machine, minimum-history criteria, selected profile,
and repositories used for each lane.

The **regularity and engineering gate** initially proposes:

- byte-identical output over repeated runs with the same inputs;
- zero inferred candidates on designated young/template negative controls when
  profiles are disabled;
- no more than 10 displayed candidates per cutoff;
- at least 5% coverage of eligible future antecedent changes on repositories
  where candidates are emitted;
- at least 10 percentage points absolute holdout-adherence improvement over the
  frequent-consequent baseline on two development repositories;
- positive improvement over both frequent-consequent and unweighted baselines
  on the untouched holdout;
- at least 0.5 Jaccard similarity between adjacent-cutoff top-k rule sets unless
  a documented repository epoch boundary explains the change;
- completion within the frozen runtime and memory budgets.

For a candidate `A -> B`, a holdout opportunity is an eligible transaction
containing `A`; it adheres when that same transaction contains `B` and deviates
otherwise. Report micro- and macro-averaged adherence separately. Coverage is
the fraction of eligible holdout transactions containing at least one
antecedent from the displayed candidate set. Every baseline uses the identical
opportunity set, cutoff, display limit, and aggregation.

These definitions and numbers are provisional and require human ratification
before implementation. Regularity metrics cannot compensate for failure of the
product-relevance gate. Once frozen, neither gate can be weakened after seeing
holdout results.

### 5.4 Stop conditions

Do not proceed to ratification or enforcement if:

- maintainers mostly classify candidates as accidents or unclear;
- sampled deviations are usually benign rather than review-worthy;
- ratifying useful candidates requires extensive code archaeology or costs as
  much as authoring the same repository contracts manually;
- results reproduce co-change but not any predeclared documented contract;
- apparent performance disappears against the frequent-consequent baseline;
- results depend on one repository or one hand-selected cutoff;
- useful adherence is achieved only with negligible coverage;
- top rules are dominated by broad infrastructure components or obvious
  implementation/test component pairs with no actionable value;
- rules are unstable under small cutoff or filter changes;
- resource use is impractical on the primary benchmark;
- the holdout fails after development thresholds have been frozen.

## 6. Repository and package structure

The initial repository shape is:

```text
.
├── cmd/
│   └── tacita/
│       └── main.go
├── internal/
│   ├── backtest/
│   ├── gitlog/
│   ├── mining/
│   └── report/
├── testdata/
├── go.mod
├── PRODUCT_BRIEF.md
└── IMPLEMENTATION_PLAN.md
```

The package responsibilities are:

- `cmd/tacita`: argument parsing, signal-aware context, dependency wiring, one
  boundary for user-facing errors and exit codes;
- `internal/gitlog`: invoke Git, parse its stream, normalize transactions, and
  expose ingestion diagnostics;
- `internal/mining`: pure aggregation and descriptive metric calculation;
- `internal/backtest`: temporal splits, baseline evaluation, and go/no-go
  metrics;
- `internal/report`: stable schema types, deterministic JSON, and text
  rendering.

This uses official Go's compiler-enforced `internal` boundary without turning
it into a junk drawer. Packages stay one level deep and are created only when
their responsibility is independently testable.

Concrete types come first. Interfaces are defined by consumers only when a real
substitution is required. Standard interfaces such as `io.Reader` and
`io.Writer` are preferred over project-specific abstractions.

## 7. CLI contract

The first CLI may expose one experimental command surface:

```text
tacita backtest [flags] <repository>
```

It uses a dedicated `flag.FlagSet`, not global flags. Required inputs include:

- repository path;
- revision, defaulting explicitly to `HEAD`;
- training cutoff or cutoff schedule;
- collection-window length;
- temporal half-life;
- maximum paths per commit;
- maximum components per commit;
- minimum raw opportunity count;
- weighting mode;
- output format;
- resource budgets.

The process:

- handles interrupt and termination through a signal-aware context;
- writes the selected report format to stdout and diagnostics to stderr through
  injected writers;
- prints each error once;
- distinguishes usage errors, invalid/incomplete repository history, resource
  budget exhaustion, Git failure, and internal invariant failure;
- never changes the repository.

The first slice does not write report files. Callers may redirect stdout if
they want persistence. This removes output/input path collisions and keeps the
read-only repository contract unambiguous.

Subcommands, Cobra, and Viper remain deferred until command complexity or
configuration precedence demonstrates a need.

### 7.1 First-product command boundary

If both evidence lanes pass, the separately approved first product exposes the
same offline behavior locally and in any CI:

```text
tacita propose [--profile <id> ...] --output tacita.proposed.yml <repository>
tacita ratify --proposals tacita.proposed.yml --manifest tacita.yml
tacita check --base <revision> --head <revision> <repository>
```

`tacita propose` emits inferred and explicitly selected profile candidates with
their distinct provenance. It may update only the proposals file and never
modifies the ratified manifest.

`tacita ratify` presents one candidate at a time with supporting evidence,
historical exceptions or profile applicability, and the choices `accept`,
`reject`, and `defer`. Rank and weighting mode remain hidden during the
decision. Only an explicit final confirmation may update `tacita.yml`; the update
must be atomic, preserve unrelated manual rules, and reject identifier or
concurrent-edit conflicts rather than overwrite them. Rejected and deferred
decisions remain review evidence but never become enforced rules.

`tacita check`:

- reads only ratified repository contracts;
- resolves explicit base and head revisions safely, then evaluates only changes
  introduced by that diff;
- reports new deviations with contract source, evidence, and exceptions;
- treats a newly ratified rule as `warn` by default; warnings do not fail CI,
  while only a rule explicitly set to `block` may produce the documented
  policy-failure exit code;
- never promotes `warn` to `block` automatically based on age, confidence,
  historical adherence, or an absence of recent deviations;
- emits versioned JSON or deterministic text through stdout and diagnostics
  through stderr;
- uses a documented exit-code contract suitable for CI;
- does not call a forge API, infer provider-specific environment variables,
  mutate the repository, or access the network.

GitHub Actions, PR annotations, pre-push hooks, and other wrappers may invoke
this command later, but they are not the product boundary or a security
boundary.

## 8. Git ingestion contract

### 8.1 Safe process execution

- Use `exec.CommandContext`; never use `sh -c`, command interpolation, or
  user-built shell fragments.
- Pass user paths and revisions as separate arguments. Resolve the requested
  revision first with `git rev-parse --verify --end-of-options
  <revision>^{commit}`, validate the returned full object ID, then use only that
  object ID in history commands. A `--` path separator does not make an
  untrusted revision safe.
- Request NUL-delimited machine output. Newlines, tabs, leading dashes, spaces,
  and non-UTF-8 path bytes must not corrupt record boundaries.
- Run an allowlisted set of Git subcommands with `LC_ALL=C`, `TZ=UTC`,
  `GIT_OPTIONAL_LOCKS=0`, `GIT_CONFIG_NOSYSTEM=1`,
  `GIT_CONFIG_GLOBAL=/dev/null`, and disabled paging. Supply command-specific
  flags that disable external diff/text-conversion behavior.
- Do not run hooks, filters, difftools, pagers, or user-defined commands.
- Do not access the network.
- On cancellation, wait for the child process and return a contextual error.

### 8.2 History definition

The experiment reads non-merge commits reachable from an explicit revision.
It uses committer timestamps for ordering and a deterministic object-ID
tie-breaker. Every run records:

- repository identity supplied by the caller;
- analyzed revision and resolved commit ID;
- earliest/latest included timestamp;
- explicit `as_of` reference time;
- filtering and weighting parameters;
- Git and Tacita versions;
- shallow-repository status.

The tool rejects shallow history by default because silently treating it as
complete would invalidate opportunity and decay calculations. An explicit
research-only override may annotate the report as incomplete.

### 8.3 Transaction normalization

Each eligible commit becomes one transaction:

- deduplicate paths;
- classify additions, modifications, deletions, and renames;
- exclude merges in the first slice;
- exclude commits above the path budget;
- exclude or separately report rename-heavy commits;
- exclude known vendored, generated, lock, binary, fixture, and golden paths
  according to explicit rules;
- retain diagnostics for every exclusion reason.

Generated-file header detection against historical blobs is not "free." The
first implementation must either inspect the blob at the relevant commit under
a strict byte budget or document that it is using name/path heuristics only.

Rename identity across time is a separate correctness problem. Until a tested
identity model exists, reports must expose how renames were treated and must not
claim continuity that was not reconstructed.

### 8.4 Component projection

Changed file paths are retained as evidence, but they are not candidate rule
identifiers. After path-level exclusions:

- each eligible path maps to its repository-relative parent directory;
- a directory containing Go files is labelled as a Go package directory without
  inferring semantics from package contents;
- the repository root is a valid component;
- paths in the same component are deduplicated for mining;
- a transaction touching fewer than two components creates no pair candidate;
- candidate `A -> B` identifiers are stable repository-relative component
  paths, while reports link back to the supporting commits and files.

This deliberately asks whether changes cross repository components in a stable
way. It does not predict the next file to edit or infer import direction.
Milestone 0 must freeze treatment of deleted-only directories, renames across
components, nested modules, submodules, and vendored trees so two implementers
produce the same projection.

### 8.5 Resource limits

All of these are explicit, validated, and reported:

- maximum commits scanned;
- maximum paths per commit;
- maximum unique paths;
- maximum components per commit;
- maximum unique components;
- maximum directional component pairs;
- maximum Git output bytes;
- maximum elapsed time;
- maximum report rows and output bytes.

Budget exhaustion is a typed failure with partial diagnostics, never silent
truncation or a success-shaped report.

## 9. Descriptive mining model

For a transaction `t`, let:

- `P(t)` be its deduplicated eligible path-change evidence;
- `C(t)` be the deduplicated component set projected from `P(t)`;
- `d(t)` be exponential temporal decay relative to the explicit training
  cutoff, never the machine's current time;
- `s(t)` be the selected size-weight experiment;
- `w(t) = d(t) * s(t)`.

For directional component candidate `A -> B`:

```text
raw_support     = count(t where A in C(t) and B in C(t))
raw_opportunity = count(t where A in C(t))

weighted_support  = sum(w(t) where A in C(t) and B in C(t))
weighted_exposure = sum(w(t) where A in C(t))
weighted_confidence = weighted_support / weighted_exposure

weighted_prevalence(B) =
  sum(w(t) where B in C(t)) / sum(w(t) over eligible transactions)

weighted_lift = weighted_confidence / weighted_prevalence(B)
```

Candidates require a minimum raw opportunity count even when their weighted
exposure is high. All divisions define zero-denominator behavior explicitly.
Floating-point output uses a fixed representation, and final ordering uses
stable tie-breakers ending in antecedent and consequent component-path bytes.

The miner begins sequentially. Candidate aggregation must be benchmarked before
introducing goroutines because concurrency can add nondeterminism and memory
pressure without improving a Git-I/O-bound pipeline.

## 10. Temporal backtest

### 10.1 Evaluation shape

Use rolling chronological cutoffs:

1. collect and train only on transactions available at cutoff `T`;
2. freeze candidates and ranks;
3. evaluate on a later interval `(T, T+n]`;
4. advance the cutoff and repeat;
5. never reuse future outcomes to adjust an earlier candidate.

Decay age is calculated relative to each training cutoff. Using the eventual
run date would leak future time and make historical results drift.

### 10.2 Metrics

Report by repository, cutoff, weighting mode, and baseline:

- eligible transactions and antecedent opportunities;
- candidate count and displayed count;
- component-level rules with supporting commit and file evidence;
- micro- and macro-averaged holdout adherence and deviation rate;
- coverage and abstention;
- lift over each baseline;
- top-k Jaccard stability across adjacent cutoffs;
- rank correlation for surviving rules;
- blinded candidate-intent and sampled-deviation labels, kept distinct from
  computed metrics;
- exclusion counts by reason;
- Git bytes and component-pair count.

The operational sidecar reports runtime and peak memory by repository, cutoff,
and weighting mode. They are excluded from the canonical analysis report
because volatile measurements cannot satisfy its byte-identity contract.

Do not optimize one aggregate score. Precision, coverage, stability, and cost
remain visible so a model cannot hide abstention behind high adherence.
Computed regularity and human product-relevance labels must never be collapsed
into one opaque score.

### 10.3 Corpus discipline

Before implementation, record immutable commit IDs for:

- development repositories used to alter filters and weights;
- young/template negative controls;
- one untouched holdout repository;
- one scale/stress repository.

Candidate repositories from the historical brief may be used only after
checking current availability, license, clone size, history shape, and whether
they use squash merges. Repositories used for threshold tuning cannot later be
called holdouts.

## 11. Report contract

JSON is the machine contract because it is available in the standard library.
The first schema includes:

- `schema_version`;
- run metadata and resolved inputs;
- completeness and shallow-history status;
- configuration and resource budgets;
- ingestion diagnostics;
- candidate rows with explicit `origin`, profile/rule identifier when
  applicable, source identifier/version, and origin-appropriate evidence;
- for inferred candidates: raw and weighted measures, supporting transactions,
  and historical exceptions;
- for profile candidates: applicability evidence, rationale, opinionated/core
  classification, current compliance state, and exact rule-level provenance;
- blinded review labels for both origins;
- cutoff and baseline results;
- stop/gate outcomes;
- warnings and non-fatal limitations.

Text output is a deterministic rendering of the same model, not a second source
of truth. No map is rendered without sorted keys. The canonical analysis report
contains no wall-clock duration, process ID, temporary path, or peak-memory
observation and must be byte-identical over repeated runs with identical
inputs.

Benchmarks and experiment orchestration may emit a separate operational
sidecar containing runtime and memory observations. It is evidence, not part of
the canonical report contract, and is not subject to byte equality.

The schema is versioned now so a future TUI can consume it without scraping
terminal prose.

## 12. Error and observability policy

- Wrap errors with operation and relevant safe identifiers using `%w`.
- Do not both log and return the same error below the CLI boundary.
- Do not expose arbitrary Git stderr without context or a size cap.
- Distinguish invalid user input from repository incompleteness and operational
  Git failures.
- Emit exclusion counts and analysis limitations; silence must be explainable.
- Never turn malformed input, budget exhaustion, or cancellation into an empty
  successful report.
- Avoid global loggers. Pass a `*slog.Logger` only if structured diagnostics
  become necessary; deterministic reports are not logs.

## 13. Security model

### 13.1 Assets and trust boundary

The analyzed repository is untrusted input. Its paths, commit metadata, blobs,
object counts, local Git configuration, and history shape may be adversarial.

### 13.2 Primary risks

- command or option injection through paths and revisions;
- parser confusion through control characters or non-UTF-8 names;
- CPU/memory/disk exhaustion through enormous histories or pair explosion;
- unexpected executable behavior induced by Git configuration;
- history incompleteness presented as valid evidence;
- symlink/path traversal if working-tree files are inspected;
- nondeterminism from time, map order, locale, or concurrency;
- supply-chain risk from unnecessary modules and build tooling.

### 13.3 Controls

- argument-array process execution and NUL framing;
- explicit budgets and cancellation;
- no repository mutation and no network;
- minimal environment and disabled pager/external behavior;
- blob reads through Git object IDs rather than arbitrary working-tree path
  traversal where practical;
- standard library first;
- pinned Go patch releases and reviewed dependency changes;
- `govulncheck`, race testing, fuzzing, and reproducible build settings;
- deterministic output tests.

Security claims are limited to these controls. A local CLI running with the
user's privileges is not a sandbox.

## 14. Testing and quality gates

### 14.1 Unit tests

- table-driven tests for transaction eligibility and exclusion reasons;
- table-driven tests for path-to-component projection and deduplication;
- exact tests for every weighting formula and zero-denominator case;
- stable-order tests with tied floating-point values and unusual path bytes;
- backtest tests proving that future transactions cannot enter training;
- report schema and rendering tests;
- error classification and cancellation tests.

### 14.2 Integration tests

Use `t.TempDir` and the real Git executable to create disposable repositories.
Cover:

- singleton and empty eligible commits;
- spaces, tabs, newlines, leading dashes, and non-UTF-8 filenames;
- additions, modifications, deletions, renames, and rename-heavy commits;
- merge histories;
- shallow clones;
- generated and binary files;
- oversized commits and each resource budget;
- equal timestamps and deterministic tie-breaking;
- cancellation of an active Git process;
- byte-identical repeated reports.

No container is needed for this boundary. Add one OCI smoke test only if OCI
packaging is later approved.

### 14.3 Fuzzing

Native Go fuzz targets cover:

- the NUL-delimited Git stream parser;
- path/status record decoding;
- transaction normalization;
- component projection;
- JSON report decoding and schema invariants.

Targets are fast, deterministic, state-independent, and have seed cases for the
integration-test edge conditions.

### 14.4 Static and dynamic gates

The initial gate set is:

```text
gofmt check
go vet ./...
go test ./...
go test -race ./...
golangci-lint run
govulncheck ./...
bounded fuzz jobs
deterministic golden-report comparison
benchmarks against frozen corpus snapshots
```

golangci-lint uses a pinned 2.x configuration with an explicit high-signal
linters list and zero warnings. Do not use `enable-all`; churn and false
positives are not rigor.

Go patch releases must be current. At planning time the workstation has Go
1.26.5 while the official current release is 1.26.6.

Release builds use `CGO_ENABLED=0` and `-trimpath`; race-test builds separately
enable the platform C toolchain required by Go's race detector. Development
tools such as golangci-lint and govulncheck are build/validation tooling, not
runtime module dependencies.

## 15. Concurrency and Go learning checkpoints

The project is also a deliberate Go learning exercise, but learning must not
force abstractions into production.

### Checkpoint A: package boundaries

Before adding a package, explain its single responsibility, why the existing
package cannot own it, and what dependency direction results.

### Checkpoint B: interfaces

Start with concrete types. Add an interface only at the consumer when:

- two implementations genuinely need substitution; or
- a narrow I/O boundary cannot be tested adequately with standard interfaces.

Review whether `io.Reader`, `io.Writer`, `fs.FS`, or a function value already
solves the need. Return concrete types.

### Checkpoint C: concurrency

Before any goroutine is merged, record:

- who starts it;
- who owns and closes each channel;
- how cancellation reaches it;
- how concurrency is bounded;
- how every send can complete;
- how errors propagate;
- how shutdown is awaited;
- how deterministic output is preserved.

Benchmark the sequential version first. The likely first valid concurrency
exercise is evaluating independent repositories or cutoffs with a bounded
fan-out, not parallel mutation of one candidate map.

### Checkpoint D: tests

Walk through why each table-driven test dimension matters, how fuzz seeds differ
from table cases, and why integration tests use real Git rather than mocks.

### Checkpoint E: errors and context

Trace one cancellation and one malformed-history error from Git process to CLI
exit. Confirm errors are wrapped once per abstraction boundary and printed once.

## 16. Milestones

### Milestone 0: Freeze the experiment

Deliver:

- ratified numeric go/no-go thresholds;
- exact corpus commit IDs and roles;
- report schema v1;
- Git history semantics;
- initial resource budgets;
- weighting modes and baselines;
- product-relevance review protocol and numeric acceptance thresholds;
- initial profile identifiers, pinned source revisions, selected rules, exact
  semantics, rule-level provenance text, applicability predicates, and
  independent opt-in behavior;
- threat model and explicit exclusions.

Exit criterion: two implementers could independently run the same experiment
without making hidden design choices.

Milestone 1 is blocked until this exit criterion is met. In particular, corpus
IDs, numeric resource budgets, report schema fields, module path, platform
support, and the provisional go/no-go numbers are unresolved Milestone 0
decisions rather than defaults for an implementer to invent.

### Milestone 1: Safe Git ingestion

Deliver:

- stdlib CLI shell;
- cancellation-aware Git runner;
- NUL stream parser;
- normalized transactions and diagnostics;
- integration and fuzz coverage for adversarial repositories.

Exit criterion: deterministic transaction output, complete cleanup, clear
shallow-history behavior, and enforced budgets.

### Milestone 2: Descriptive miner

Deliver:

- path-to-component projection and directional component-pair aggregation;
- raw and weighted measures;
- stable ranking and report rows;
- weighting-mode comparison;
- unit tests and memory/runtime benchmarks.

Exit criterion: exact repeated results and bounded behavior on the development
corpus.

### Milestone 3: Temporal backtest

Deliver:

- rolling cutoff evaluation;
- all trivial baselines;
- blinded candidate-intent and sampled-deviation assessment;
- comparison with preregistered documented repository contracts;
- development, negative-control, and stress reports;
- untouched holdout run after thresholds are frozen;
- reproducible evidence bundle.

Exit criterion: complete go/no-go evidence without changing thresholds after
the holdout is inspected.

### Milestone 4: Cold-start profile evaluation

Deliver:

- the frozen `go-core` and `spf13-idiomatic` rule sets and independent opt-in
  selection;
- deterministic profile candidate evaluation on new repositories;
- proposals for applicable rules regardless of current compliance, with
  explainable applicability and current-state diagnostics;
- origin and source/version provenance in every output;
- a time-boxed ratification study on the new-repository corpus;
- evidence that profile candidates never affect inferred metrics or gates.

Exit criterion: at least three profile-origin expectations can be ratified
within 10 minutes on the frozen new-repository evaluation corpus without
misrepresenting them as repository-derived.

### Milestone 5: Human decision

Choose one:

1. if both origin lanes pass, proceed to a separately planned first product
   containing proposal, ratification, and enforcement;
2. revise a failed lane and repeat it with a new preregistered experiment;
3. stop because the combined product claim is not actionable.

No enforcement code starts automatically.

## 17. Deferred roadmap

The following order is provisional and requires separate approval:

1. human ratification model and manifest format;
2. diff-only enforcement and baseline/ratchet semantics;
3. optional provider-specific CI wrappers and review annotations;
4. expansion or additional opt-in profiles with explicit provenance;
5. import-graph rules with a separately defined directed analysis;
6. TUI consuming report schema v1+;
7. optional MCP server isolated from the offline core;
8. generated documentation from ratified machine policy;
9. AST transaction experiments;
10. release automation and optional minimal Git-capable OCI image;
11. Kubernetes policy/CRD work only if a real deployment use case exists.

This preserves the screenshots' visible principle: make the proposal/report
model reusable from the beginning, then add presentation layers later.

## 18. Critical risks

| Risk | Mitigation or stop rule |
| --- | --- |
| Co-change reflects workflow, not architecture | Trivial baselines, qualitative labels, holdout, no causal claims |
| High apparent precision comes from silence | Report coverage and abstention beside adherence |
| Ratification costs as much as writing rules | Time-box reviews; measure decision burden and useful ratifications per session |
| Tangled/large commits dominate | Hard budgets, weighting experiment, exclusion diagnostics |
| Common files correlate with everything | Lift, explicit exclusions, frequent-consequent baseline |
| Temporal leakage inflates results | Cutoff-relative decay and chronological evaluation |
| Thresholds overfit public repos | Separate development and untouched holdout repositories |
| Rename/history semantics create fake drift | Explicit rename policy and completeness metadata |
| Pair count exhausts memory | Candidate budgets, benchmarks, typed budget failure |
| Concurrency adds races/nondeterminism | Sequential baseline, ownership review, race tests |
| "Strict linting" becomes noise | Explicit high-signal list, no `enable-all` |
| Scratch image drives a Git dependency rewrite | Native binary first; Git-capable runtime only if needed |
| Profiles undermine repository-local positioning | Opt-in, provenance-labelled, separately gated, never reported as inferred |
| Opinion is presented as Go consensus | Separate `go-core` from `spf13-idiomatic`; cite every rule and never enable either implicitly |
| Mining and profiles become two unrelated products | Require the same proposal/ratification/check loop, measure value by origin, and split the roadmap if users perceive different jobs |
| Ratification launders correlation into policy | Show evidence/exceptions and keep human intent explicit |

## 19. Strongest opposing argument

Tacita's central thesis may be wrong even if this implementation is flawless.
Co-change mining may discover only commit conventions and obvious
implementation/test pairs. High holdout adherence would then validate a
predictor, not a product. Human ratification cannot transform correlation into
causality, and aggressive abstention can make a low-value tool appear precise.
If maintainers do not recognize candidates as project decisions, their
deviations as review-worthy, or the proposal workflow as cheaper than authoring
contracts manually, the correct result is to stop—even if the miner beats
simple baselines. Do not let profile candidates hide a failed inferred lane,
expand the profile to manufacture a pass, lower thresholds, or build
enforcement around weak product evidence.

The strongest competing interpretation is that Tacita now bundles a history miner
with a conventional Go policy pack. Profiles may solve cold start while also
making the differentiated mining path unnecessary or confusing. The combined
product claim therefore survives only if reviewers understand both origins as
inputs to the same ratification job and derive distinct value from each. If
profile users want a linter while mining users want a change-coupling analysis
tool, the correct response is to split or remove a lane rather than conceal the
division behind one manifest.

This argument changes Tacita from an assumed product architecture into a
falsifiable experiment and weakens confidence in the combined first-product
scope. It does not change the decision to specify both lanes in Milestone 0,
because their gates and provenance are now independent.

## 20. Source and guidance precedence

Sources used for this plan:

- [`PRODUCT_BRIEF.md`](PRODUCT_BRIEF.md);
- the two cropped screenshots in this directory, as supplemental input only;
- [`spf13/go-skills`](https://github.com/spf13/go-skills), reviewed at
  `e67851cfcca008592c7c4965b8220c7cb37e2f1c`;
- [official Go module layout](https://go.dev/doc/modules/layout);
- [official Go security practices](https://go.dev/doc/security/best-practices);
- [official Go fuzzing](https://go.dev/doc/security/fuzz/);
- [official Go race detector](https://go.dev/doc/articles/race_detector);
- [current official Go version](https://go.dev/VERSION).

The installed upstream Go skill describes itself as current through Go 1.25,
while this project targets Go 1.26. Official Go documentation, the selected
toolchain, repository-local instructions, measured evidence, and explicit human
decisions take precedence over opinionated or stale skill guidance.

Repository or GitHub publication requires a separate explicit decision.
