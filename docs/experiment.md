# Experiment Protocol

This document defines the evidence required before Tacita becomes an
enforcement product. Numeric thresholds, corpus revisions, and resource budgets
remain provisional until Milestone 0 freezes them.

## Hypotheses

### Repository-inferred candidates

For repositories with sufficient coherent history, Tacita can surface a small
set of stable, sparse, deterministic, and explainable candidate expectations
that:

1. remain regularities on unseen history rather than frequency artifacts; and
2. maintainers recognize as repository decisions whose deviations merit
   review.

The second condition is the product hypothesis. Predictive co-change behavior
tests only the first.

### Profile candidates

For repositories without useful history, independently selected profiles can
propose at least three applicable expectations that a maintainer chooses to
ratify within the same adoption budget.

Profile evidence is curated guidance, not repository inference. Profile success
cannot compensate for a failed history-mining hypothesis.

## Candidate origins

Every candidate retains one origin through proposal, review, and reporting:

- `inferred`: a directional relationship between repository components,
  supported by historical commits and file changes;
- `profile`: a deterministic rule from an explicitly selected, versioned
  profile.

The initial profile catalog is:

| Tier | Rule IDs |
| --- | --- |
| `go-core` | `module-topology`, `go-version-floor`, `release-no-local-replace`, `semantic-import-version` |
| `spf13-idiomatic` | `no-root-pkg`, `domain-over-layers`, `max-package-depth`, `no-heavy-test-frameworks` |

`go-core` is grounded in official
[Go module documentation](https://go.dev/doc/modules/).
`spf13-idiomatic` is selected from the pinned
[`spf13/go-skills`](https://github.com/spf13/go-skills/tree/e67851cfcca008592c7c4965b8220c7cb37e2f1c)
revision.

The tiers are enabled independently. Applicable profile rules are proposed even
when the repository currently complies, because ratification establishes a
future contract. Exact parser and applicability semantics are a Milestone 0
blocker.

## Historical transaction model

Each eligible non-merge commit becomes a transaction:

1. retain normalized path changes as evidence;
2. apply explicit path and commit exclusions;
3. map each path to a stable repository-relative component;
4. deduplicate components within the commit;
5. emit directional component pairs only when at least two components remain.

Files are evidence, not rule identifiers. The initial projection uses parent
directories and may label directories containing Go files as package
directories without inferring package semantics.

Deleted-only directories, cross-component renames, nested modules, submodules,
vendored trees, and symlinks require frozen behavior before implementation.

## Descriptive model

For transaction `t`:

```text
P(t) = deduplicated eligible path evidence
C(t) = deduplicated projected components
d(t) = temporal decay relative to the training cutoff
s(t) = selected size-weight mode
w(t) = d(t) * s(t)
```

For directional candidate `A -> B`:

```text
raw_support     = count(t where A and B are in C(t))
raw_opportunity = count(t where A is in C(t))

weighted_support    = sum(w(t) where A and B are in C(t))
weighted_exposure   = sum(w(t) where A is in C(t))
weighted_confidence = weighted_support / weighted_exposure

weighted_prevalence(B) =
  sum(w(t) where B is in C(t)) / sum(w(t) over eligible transactions)

weighted_lift = weighted_confidence / weighted_prevalence(B)
```

Candidates require a minimum raw opportunity count. Every report keeps raw
counts beside weighted values and defines zero denominators, float formatting,
and byte-level tie-breakers.

The experiment compares:

1. unit weight after the hard commit-size filter;
2. inverse component size: `1 / max(1, |C(t)| - 1)`;
3. pair-normalized weight:
   `1 / max(1, |C(t)| * (|C(t)| - 1))`.

These are descriptive weighted sums. Wilson intervals, Fisher tests,
Benjamini-Hochberg corrections, statistical significance, and FDR claims are
invalid over the fractional observations and are excluded.

## Temporal evaluation

Use rolling chronological cutoffs:

1. train only on transactions available at cutoff `T`;
2. freeze candidates and ranks;
3. evaluate them on `(T, T+n]`;
4. advance the cutoff and repeat.

Decay is relative to each training cutoff, never the machine clock. A holdout
opportunity for `A -> B` is an eligible transaction containing `A`; it adheres
when the same transaction contains `B`.

Report:

- micro- and macro-averaged adherence and deviation;
- coverage and abstention;
- candidate count and display count;
- lift over each baseline;
- adjacent-cutoff top-k Jaccard stability;
- rank correlation for surviving rules;
- exclusion counts and bounded-work diagnostics;
- human labels separately from computed metrics.

Do not collapse regularity, relevance, coverage, stability, and cost into one
score.

## Baselines and controls

Every inferred experiment uses the same opportunity set, cutoff, display limit,
and aggregation for:

1. frequent consequent, ignoring the antecedent;
2. unweighted co-change;
3. a time-shuffled control with a fixed seed and stable PRNG;
4. a no-history negative control.

With profiles disabled, young repositories must not produce fabricated
inferred certainty. Enabled profiles are reported in a separate lane.

## Product-relevance review

Before mining:

- preregister documented repository contracts that the method could plausibly
  rediscover;
- freeze repositories, revisions, reviewer eligibility, samples, thresholds,
  and timing rules.

After mining:

- hide rank and weighting mode;
- show supporting changes and historical exceptions;
- label candidates `decision`, `accident`, or `unclear`;
- label sampled holdout deviations `review-worthy`, `benign`, or `unclear`;
- record evidence inspected, decision time, and reviewer relationship to the
  repository.

Reviewers must already understand the evaluated repository. Mining time counts
toward the adoption budget. The target remains three useful ratifications
within ten minutes for each evaluated origin lane, subject to Milestone 0
ratification.

## Provisional engineering gate

Milestone 0 must accept or replace these values before holdout inspection:

- byte-identical canonical output for identical inputs;
- zero inferred candidates on designated negative controls;
- at most 10 displayed candidates per cutoff;
- at least 5% future-opportunity coverage when candidates are emitted;
- at least 10 percentage points adherence improvement over the
  frequent-consequent baseline on two development repositories;
- positive improvement over frequent-consequent and unweighted baselines on
  the untouched holdout;
- at least 0.5 adjacent-cutoff top-k Jaccard similarity unless a preregistered
  epoch boundary explains the change;
- completion within frozen runtime and memory budgets.

Product-relevance thresholds remain unresolved and cannot be replaced by these
regularity metrics.

## Corpus discipline

Freeze immutable commit IDs for:

- development repositories used to tune filters and weights;
- young/template negative controls;
- one untouched holdout;
- one scale/stress repository;
- mature and new repositories with qualified reviewers for adoption timing.

Record availability, license, clone size, history shape, and merge strategy.
Anything used for tuning cannot later be called a holdout.

## Report evidence

The canonical JSON report is versioned and deterministic. It contains resolved
inputs, completeness, configuration, budgets, ingestion diagnostics,
origin-specific candidates and evidence, baseline results, human labels, gate
outcomes, and limitations.

Text is rendered from the same model. Map-derived data is sorted. Wall-clock
duration, process IDs, temporary paths, and peak memory belong in a separate
operational sidecar because they would break byte identity.

## Stop conditions

Stop, revise, or split the product if:

- maintainers mostly label candidates accidental or unclear;
- sampled deviations are usually benign;
- ratification costs about as much as manually writing contracts;
- results do not rediscover any preregistered repository decision;
- performance disappears against trivial baselines;
- useful adherence depends on negligible coverage or one repository;
- small cutoff/filter changes destabilize the rules;
- resource use exceeds the frozen budget;
- the untouched holdout fails.

Thresholds must not be weakened after holdout results are known.

## Strongest opposing argument

Co-change may reveal commit habits and obvious implementation/test pairs rather
than architecture. High holdout adherence can validate a predictor without
creating a useful review product, while abstention can make a weak tool appear
precise. Human ratification does not turn correlation into causality.

Tacita may also be combining two products: a history miner and a conventional
Go policy pack. The shared workflow is coherent only if maintainers experience
both origins as inputs to the same ratification job and gain distinct value
from each. Otherwise the correct result is to remove or split a lane, not hide
the difference behind one manifest.
