# Experiment Protocol

This document defines the evidence required before Tacita becomes an
enforcement product. The first experiment's corpus, calibration, gates,
controls, and stop rules are frozen below.

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
| `go-core` | `module-topology`, `go-version-floor`, `no-local-replace`, `semantic-import-version` |
| `spf13-idiomatic` | `max-package-depth`, `domain-over-layers`, `no-generic-packages`, `no-heavy-test-frameworks` |

`go-core` is grounded in official
[Go module documentation](https://go.dev/doc/modules/).
`spf13-idiomatic` is selected from the pinned
[`spf13/go-skills`](https://github.com/spf13/go-skills/tree/e67851cfcca008592c7c4965b8220c7cb37e2f1c)
revision.

The tiers are enabled independently. Applicable profile rules are proposed even
when the repository currently complies, because ratification establishes a
future contract. [`profiles.md`](profiles.md) freezes parser, applicability,
identifier, and provenance semantics.

## Historical transaction model

Each eligible
[first-parent integration event](architecture.md#integration-history) becomes
a transaction. Single-parent commits, including squash results, contribute
their parent diff and merge commits contribute their net first-parent diff.
The root event is recorded but excluded from mining:

1. retain normalized path changes as evidence;
2. apply explicit path and integration-event exclusions;
3. map each path to a stable repository-relative component;
4. deduplicate components within the integration event;
5. emit directional component pairs only when at least two components remain.

Files are evidence, not rule identifiers. The frozen
[component projection](architecture.md#component-projection) uses parent
directories, excludes vendor paths and gitlinks, never follows symlinks, and
does not treat nested modules as implicit boundaries.

## Descriptive model

For transaction `t`:

```text
P(t) = deduplicated eligible path evidence
C(t) = deduplicated projected components
s(t) = selected size-weight mode
w(t) = s(t)
```

The first experiment uses no temporal decay: every training transaction has
temporal weight `1`, regardless of age. Recency weighting would introduce an
unvalidated time basis and half-life, so it requires a separately
preregistered experiment if development evidence later justifies it.

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

1. unit weight after the hard integration-event-size filter;
2. inverse component size: `1 / max(1, |C(t)| - 1)`;
3. pair-normalized weight:
   `1 / max(1, |C(t)| * (|C(t)| - 1))`.

These are descriptive weighted sums. Wilson intervals, Fisher tests,
Benjamini-Hochberg corrections, statistical significance, and FDR claims are
invalid over the fractional observations and are excluded.

## Candidate configuration grid

Inference abstains until a training partition contains at least 100 eligible
transactions. Each directional pair must have at least 3 raw supporting
transactions. The frozen grid is the Cartesian product:

| Parameter | Values |
| --- | --- |
| Size weight | `unit`, `inverse-component`, `pair-normalized` |
| Minimum raw opportunities | `5`, `10`, `20` |
| Minimum weighted confidence | `0.70`, `0.80`, `0.90` |
| Minimum weighted lift | `1.25`, `1.50`, `2.00` |

The grid contains exactly 81 configurations. A pair is eligible only when it
meets every threshold in its configuration. All metrics must be finite; a zero
or invalid denominator makes the pair ineligible.

Eligible candidates are ranked by:

1. weighted confidence, descending;
2. weighted lift, descending;
3. raw support, descending;
4. raw opportunity, descending;
5. antecedent component bytes, ascending;
6. consequent component bytes, ascending.

Each cutoff and final proposal displays the first 10 candidates, or every
eligible candidate when fewer than 10 exist. Candidate identity is the ordered
tuple `(origin, antecedent, consequent)`.

## Temporal evaluation

Use expanding-history rolling evaluation over the immutable first-parent
integration history ending at the frozen repository revision. Let `E1 ... EN`
be all integration events from root to tip before path, size, or other
eligibility exclusions. For `p` in `60, 70, 80, 90`, define:

```text
b(p) = floor(p * N / 100)
C(p) = object ID of event E[b(p)]
```

Each cutoff uses the following fixed partition:

| Cutoff | Training events | Evaluation events |
| --- | --- | --- |
| `C(60)` | `E1 ... E[b(60)]` | `E[b(60)+1] ... E[b(70)]` |
| `C(70)` | `E1 ... E[b(70)]` | `E[b(70)+1] ... E[b(80)]` |
| `C(80)` | `E1 ... E[b(80)]` | `E[b(80)+1] ... E[b(90)]` |
| `C(90)` | `E1 ... E[b(90)]` | `E[b(90)+1] ... EN` |

Only eligible transactions within each fixed partition contribute to mining
or evaluation. Defining boundaries before exclusions prevents a filter or
weighting variant from changing the periods being compared. Evaluation windows
do not overlap.

At each cutoff:

1. train on eligible transactions in its training partition;
2. freeze candidates and ranks;
3. evaluate them on eligible transactions in its evaluation partition.

Training membership is cumulative:

```text
train(C(60)) is a subset of train(C(70))
train(C(70)) is a subset of train(C(80))
train(C(80)) is a subset of train(C(90))
```

No earlier transaction leaves training as the cutoff advances. An eligible
revert is a new integration event; it does not remove the event it reverts.
All retained transactions keep the same temporal weight as the cutoff
advances; the machine clock never affects mining.

A holdout opportunity for `A -> B` is an eligible evaluation transaction
containing `A`; it adheres when the same transaction contains `B`.

Adjacent-cutoff stability compares candidate sets, not transaction sets. For
displayed top-`k` candidate identity sets at consecutive cutoffs:

```text
Jaccard = |K(C(p)) intersect K(C(p+10))|
          / |K(C(p)) union K(C(p+10))|

p in {60, 70, 80}
```

Jaccard measures membership churn; rank correlation separately measures order
changes among candidates present at both cutoffs.

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

## Evaluation metrics

For displayed candidate `r = A -> B` in evaluation partition `W`:

```text
opportunities(r, W) = count(t in W where A is in C(t))
adherences(r, W)    = count(t in W where A and B are in C(t))
adherence(r, W)     = adherences(r, W) / opportunities(r, W)
```

Candidate-cutoff adherence is `null` when its opportunity count is zero.

For one repository:

- **micro adherence** pools adherence and opportunity counts over all displayed
  candidate-cutoff pairs with opportunities;
- **macro adherence** is the arithmetic mean of their individual adherence
  values, giving each candidate-cutoff pair equal weight;
- **coverage** is the fraction of eligible evaluation transactions containing
  the antecedent of at least one displayed candidate;
- **cutoff abstention** is the fraction of the four cutoffs that display no
  candidate;
- **minimum stability** is the minimum of the three adjacent-cutoff Jaccard
  values;
- **rank correlation** is Kendall's tau-b over candidates present in both
  adjacent rankings and is diagnostic rather than a gate.

Micro, macro, and coverage pool the four non-overlapping evaluation
partitions. A repository metric is `null` when its denominator is zero.

## Baselines and controls

Every inferred experiment uses the same partitions, display limit, and metric
aggregation for:

1. **Frequent consequent:** for each displayed candidate slot with antecedent
   `A`, replace its consequent with the component having the greatest raw
   training-transaction prevalence other than `A`; break prevalence ties by
   component bytes. This preserves the model's antecedent opportunities while
   ignoring association with `A`.
2. **Unweighted co-change:** rerun the complete candidate pipeline with the
   selected configuration's thresholds but force `unit` size weight.
3. **Hash-shuffled control:** sort integration events by ascending SHA-256 of
   `tacita-shuffle-v1`, the frozen repository object ID, and the event object
   ID, encoded as UTF-8 fields joined by single NUL bytes with no trailing NUL;
   then apply the same percentile partitions. This destroys chronology without
   a runtime-dependent PRNG.
4. **No-history control:** use an empty training partition and require zero
   inferred candidates.

The Proficiency repository, with profiles disabled, is the young-repository
negative control and must abstain because it has fewer than 100 integration
events. `go-starter` is a template-origin control: its root event must be
excluded, but later history may produce candidates and is reported
diagnostically. Enabled profiles remain a separate lane.

## Development calibration

Milestone 0 freezes a finite candidate-model configuration grid and one
deterministic selection rule, not a parameter winner chosen without evidence.
It also freezes all development acceptance metrics and tie-breakers used by the
selector.

After implementation, calibration:

1. produces one calibration report per development repository, evaluating all
   81 frozen configurations with the same temporal partitions, baselines,
   controls, and budgets;
2. verifies the no-history and Proficiency negative controls and records the
   `go-starter` template-origin control;
3. selects one configuration using only the frozen rule;
4. writes the selected parameters, development metrics, canonical
   configuration bytes, and SHA-256 digest to a configuration lock;
5. prevents holdout report generation until that lock exists.

A configuration remains selectable only if every frozen development
engineering dimension and required negative control passes on both development
repositories. Holdout-only candidate-count and predictive requirements are not
calibration inputs. Among selectable configurations, compare these keys in
order:

1. greatest worst-case adherence gain over frequent consequent, taking the
   minimum across micro and macro metrics and both development repositories;
2. greatest worst-case coverage across the two development repositories;
3. greatest minimum adjacent-cutoff Jaccard across both repositories;
4. simplest size weight in the order `unit`, `inverse-component`,
   `pair-normalized`;
5. ascending canonical configuration ID.

The canonical configuration ID is:

```text
weight=<name>;opportunities=<integer>;confidence=<two decimals>;lift=<two decimals>
```

The configuration lock schema is `tacita.dev-lock/v1` and contains the corpus
object IDs, grid digest, selector version `development-selector-v1`, selected
configuration ID and parameters, development report digests, selector metric
values, and canonical lock digest. It contains no timestamps or machine-local
paths.

Calibration must not read or report Kapparmor candidates, ranks, metrics, or
labels. After the configuration lock is written, filters, weights, thresholds,
ranking, budgets, and report semantics cannot change before holdout evaluation.

If no grid configuration passes the frozen development requirements, the lane
fails without inspecting the holdout. Changing the grid or selector requires a
new preregistered experiment; the original run cannot call later evidence its
untouched holdout.

## Frozen product-relevance gate

The following protocol and thresholds were frozen on 2026-08-16 before mining
the Kapparmor holdout.

### Preregistered Kapparmor contract

The only contract counted for preregistered rediscovery is release-version
synchronization:

- a change to `APP_VERSION` or `CHART_VERSION` in `config/config` requires the
  corresponding `appVersion` or `version` change in
  `charts/kapparmor/Chart.yaml`.

Kapparmor documents this release step in its README and enforces the checked
tree relationship in `build/check_versions.sh` at the frozen commit. Because
the initial miner observes components rather than fields, successful
rediscovery means that the final displayed inferred proposal contains the
directional component candidate `config -> charts/kapparmor` and its supporting
evidence contains at least one transaction that changes the corresponding
version fields in both files. Presence of the component pair without that
evidence does not count. This check occurs only after the blinded labels are
locked. The reverse direction and conditional application, integration-test,
documentation, and changelog relationships are exploratory; they cannot
satisfy the preregistered-rediscovery condition.

### Frozen samples and blinding

After all development-corpus choices are fixed, the timed inferred run performs
these steps before the reviewer sees any holdout candidate or deviation:

1. start the uninterrupted adoption clock;
2. produce the canonical Kapparmor report at the frozen revision;
3. select the eight lowest-hash distinct candidates from the final displayed
   inferred proposal;
4. select ten distinct holdout deviations linked to those candidates,
   selecting the lowest-hash deviation for each candidate with deviations
   before filling the remaining places by hash order;
5. write the immutable review manifest and its SHA-256 digest;
6. display the first candidate to the reviewer.

The clock includes report and manifest generation. The timed run cannot reuse
candidate or manifest output cached by determinism or engineering checks.

The generated manifest contains:

1. the canonical report digest;
2. eligible and selected unit identities;
3. evidence references and selection hashes;
4. the manifest's SHA-256 digest.

Selection and presentation order use ascending SHA-256 over UTF-8 fields joined
by single NUL bytes, with no trailing NUL:

```text
inferred candidate =
  tacita-product-review-v1, repository object ID, inferred-candidate,
  inferred, antecedent bytes_b64url, consequent bytes_b64url

deviation =
  tacita-product-review-v1, repository object ID, deviation,
  inferred, antecedent bytes_b64url, consequent bytes_b64url,
  training-cutoff object ID, violating-commit object ID

profile candidate =
  tacita-product-review-v1, repository object ID, profile-candidate,
  profile, profile tier, rule ID
```

Object IDs are lowercase hexadecimal and component, tier, and rule identifiers
are their canonical report values. Selection hashes and original rank are
hidden during review.

If fewer than eight displayed candidates or ten eligible deviations exist, the
inferred product gate is `inconclusive`; a smaller sample cannot pass. No unit
may be substituted after the reviewer sees the manifest. Evidence-generation
failure invalidates the review rather than silently shrinking the sample.
The later report and engineering decisions must therefore permit at least
eight displayed inferred candidates in the experiment configuration; freezing
a lower limit would require reopening this gate before holdout inspection.

The reviewer sees the directional candidate, supporting changes, and
historical exceptions, but not its rank, weighting mode, selection hash, gate
totals, or other labels. A deviation review additionally shows the violating
transaction and the candidate evidence available at its cutoff. Evidence is
ordered canonically rather than by a relevance score.

### Labels and thresholds

Each candidate receives exactly one label:

- `decision`: an intentional repository convention that is suitable for
  ratification at `warn`;
- `accident`: a historical pattern that should not become policy;
- `unclear`: the supplied evidence is insufficient to decide.

Each sampled deviation receives exactly one label:

- `review-worthy`: the change merits maintainer attention under the candidate;
- `benign`: the change is an acceptable exception that should not alert;
- `unclear`: the supplied evidence is insufficient to decide.

The review record includes the evidence inspected, monotonic elapsed
decision time, optional rationale, and reviewer relationship to the repository.
Labels are locked before aggregate results are shown. `unclear` counts as an
unsuccessful label in both denominators; it is never discarded.

The inferred precision gates are exact counts:

- candidate precision passes with at least 6 `decision` labels among the 8
  frozen candidates;
- deviation precision passes with at least 6 `review-worthy` labels among the
  10 frozen deviations.

### Adoption timing

Adoption timing is run separately for the Kapparmor inferred lane and the
Proficiency profile lane. The repository is already cloned at its frozen
object and the frozen binary and command configuration are ready before timing;
installation, cloning, and experiment setup are excluded.

For each lane, one uninterrupted monotonic clock starts immediately before
candidate generation and stops when the reviewer accepts a third candidate for
ratification at `warn`. Candidate generation, applicability evaluation,
rendering, evidence inspection, and decision entry all count. The clock cannot
be paused. A useful inferred ratification must also be labelled `decision`; a
useful profile ratification must be applicable and retain its `profile` origin.

The timed inferred run is the reviewer's first exposure to Kapparmor candidate
content. It presents the eight frozen candidate units in hash order, records
their labels during the same session, and continues after the timer stops until
all eight are labelled. The timed profile run is likewise the first exposure to
the Proficiency profile proposals and presents applicable rules in hash order.

The lane passes adoption with at least three useful ratifications no later than
10 minutes after the clock starts. Fewer than three eligible proposals is a
failure, not abstention. An external interruption produces an `invalid` review
result and an `inconclusive` non-passing lane. The run may not be restarted
after candidate content has been seen.

The inferred product lane passes only if candidate precision, deviation
precision, adoption timing, and preregistered rediscovery all pass. The profile
lane passes only if its adoption timing passes. The overall product gate
requires both lanes; success in one cannot compensate for failure or an
inconclusive result in the other.

## Gate outcome semantics

Every required engineering dimension is evaluated independently. A composite
score or strength in one dimension cannot compensate for failure in another.

Each dimension has one outcome:

- `pass`: the metric exists and satisfies its frozen requirement;
- `fail`: valid evidence contradicts the requirement;
- `inconclusive`: the frozen corpus or partition does not provide enough
  evidence to calculate the required metric.

Both `fail` and `inconclusive` are non-passing. The overall conjunctive gate is
`fail` if any dimension fails; otherwise it is `inconclusive` if any dimension
is inconclusive; it passes only when every required dimension passes.

Examples of failure include missing a numeric threshold, exhausting a resource
budget, emitting any inferred candidate on the negative control, or emitting no
candidate on a positive lane that requires candidate evidence. Zero candidates
is a pass only where the frozen negative-control contract requires zero.

An undefined denominator is never replaced by a success-shaped number. The
metric is `null` with an explicit reason when, for example:

- a fixed evaluation partition contains no eligible events;
- no required future opportunity exists;
- both adjacent top-`k` candidate sets are empty;
- too few candidates survive in both cutoffs for rank correlation.

If one adjacent candidate set is empty and the other is not, Jaccard is the
defined value `0`. If both are empty, Jaccard is undefined; the positive lane
already fails for emitting no candidates. Exact reason identifiers belong to
the report contract.

## Frozen engineering gate

The following requirements were frozen on 2026-08-16 before development
calibration or holdout inspection.

### Determinism and completeness

- Two independent runs with identical resolved inputs, configuration lock, and
  tool versions must produce byte-identical canonical reports.
- Every integration event is either included once or reported under exactly one
  exclusion reason.
- Budget exhaustion, missing objects, unsupported environments, and undefined
  required metrics are non-passing; they cannot produce a complete report.

### Controls and candidate emission

- Empty training and profile-disabled Proficiency emit zero inferred
  candidates.
- The `go-starter` root event never contributes evidence; later candidates are
  diagnostic and do not fail the control.
- Each development and holdout cutoff displays between 1 and 10 inferred
  candidates.
- The final full-history Kapparmor proposal displays between 8 and 10 inferred
  candidates so the frozen product sample can be formed.

### Coverage and predictive behavior

- Pooled future-transaction coverage is at least 5% on each development
  repository and the holdout.
- Every evaluation partition contains at least one covered transaction.
- On each development repository, both micro and macro adherence improve by at
  least 10 percentage points over frequent consequent and are not lower than
  the unweighted co-change baseline.
- On the holdout, both micro and macro adherence improve by at least 5
  percentage points over frequent consequent and are not lower than the
  unweighted co-change baseline.
- On each development repository and the holdout, chronological micro and macro
  adherence exceed the hash-shuffled control by at least 5 percentage points.

### Stability and cost

- Every adjacent-cutoff top-10 Jaccard value is at least `0.50`; the first
  experiment has no epoch-boundary exception.
- Kendall tau-b is reported when at least two candidates survive in both
  adjacent rankings but is diagnostic rather than gating.
- Every repository run completes within all frozen resource limits.

The frozen product-relevance thresholds above cannot be replaced by these
regularity metrics.

## Frozen evaluation corpus

The first experiment uses the following corpus, frozen on 2026-08-16 before
candidate mining or holdout inspection:

| Repository | Frozen commit | Roles |
| --- | --- | --- |
| [`golang/tools`](https://github.com/golang/tools) | `d68c5293798322055c6b3ec11cf34d1e8705438f` | Development and tuning |
| [`kubernetes/kubernetes`](https://github.com/kubernetes/kubernetes) | `a231bf3f37761e8955eefba61855da1a526a3eb9` | Development, tuning, and scale/stress |
| [`allaboutapps/go-starter`](https://github.com/allaboutapps/go-starter) | `9f7a54cca5cf7b68657e6d5242bab3fcef81ae2d` | Template-origin control |
| [`tuxerrante/proficiency`](https://github.com/tuxerrante/proficiency) | `f0a8a795013de170318e6a6b879fdcd0e45cd50e` | Young-repository negative control and profile adoption timing |
| [`tuxerrante/kapparmor`](https://github.com/tuxerrante/kapparmor) | `5f9694c42c06e63bb825536486bea93b36418c5d` | Untouched holdout and inferred-candidate adoption timing |

All five repositories are anonymously available over HTTPS at the frozen
commits. The qualified reviewer for the adoption studies is `tuxerrante`, the
owner and maintainer of both adoption repositories before the freeze date.
Review is limited to areas where the reviewer has direct decision history.

The selection deliberately separates tuning from holdout evaluation.
`golang/tools` provides a medium, mostly linear history; Kubernetes supplies a
large merge-heavy history and the stress case; `go-starter` is explicitly
marked as a GitHub template and tests root-event exclusion without requiring
later abstention; Proficiency supplies a public repository below the
100-transaction inference floor; and Kapparmor supplies a multi-year repository
whose maintainer can assess intent. No result from Kapparmor may change
filters, weights, thresholds, budgets, or report semantics.

Selection-time metadata recorded from the GitHub repository and commits APIs at
the freeze:

| Repository | License | GitHub size (KiB) | Commits | Merge-history sample |
| --- | --- | ---: | ---: | --- |
| `golang/tools` | BSD-3-Clause | 58,727 | 10,693 | 0 merges in latest 100 commits |
| `kubernetes/kubernetes` | Apache-2.0 | 1,510,679 | 140,375 | 42 merges in latest 100 commits |
| `allaboutapps/go-starter` | MIT | 7,416 | 1,062 | 32 merges in latest 100 commits |
| `tuxerrante/proficiency` | Apache-2.0 | 16,455 | 51 | 1 merge in all 51 commits |
| `tuxerrante/kapparmor` | Apache-2.0 | 3,206 | 277 | 12 merges in latest 100 commits |

The API commit counts include merges, and merge-history values are descriptive
samples rather than ingestion results. GitHub-reported size is a selection-time
proxy, not a measured clone budget. Before mining, the reproducibility record
must capture the Git version, cold full-clone transferred bytes, resulting
`.git` bytes, shallow status, reachable commit count, first-parent event count,
single-parent event count, and merge-result event count for each frozen object.
Git-derived values replace API metadata in experiment reports but cannot change
corpus membership after results are seen.

The local Tacita repository at
`0174fabc9f091c6c9bfcb95f3c279d789b34c654` may be used only as an
implementation sanity fixture. It is excluded from tuning, acceptance gates,
and reported corpus results because the object was not publicly available at
the freeze. Publishing it later does not add it to this experiment; inclusion
would require a separately preregistered rerun.

## Report evidence

The frozen [`report-v1` contract](report-v1.md) separates:

- deterministic analytical reports;
- the development configuration lock;
- immutable review manifests and review results;
- nondeterministic operational sidecars;
- a final evidence index that references every artifact by digest.

Text is rendered from the analytical model without recomputing metrics.
Runtime, memory, process-local values, and human review records never mutate
canonical analytical report bytes.

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
