# Report Contract v1

This document owns the machine-readable experiment report, development lock,
review manifest, canonical encoding, and operational sidecar.

## Canonical JSON

Canonical artifacts use:

- UTF-8 JSON;
- one minified object followed by one LF byte;
- the field order specified below;
- lowercase hexadecimal SHA-1 object IDs and SHA-256 digests;
- base64url without padding for arbitrary path and component bytes;
- decimal metric strings with exactly six digits after the decimal point;
- JSON `null` plus a reason code for undefined metrics;
- no timestamps, runtime durations, process IDs, local paths, hostnames, or
  memory observations unless an artifact schema below explicitly permits the
  field.

Internal comparisons use finite `float64` values before rendering. Negative
zero renders as `"0.000000"`. NaN and infinities are forbidden.

Canonical writers use typed structs rather than maps. Any key/value collection
is rendered as an array sorted by key bytes. Unknown fields or enum values are
decoding errors for v1.

## Byte identities

Every repository byte string is represented as:

```json
{"bytes_b64url":"aW50ZXJuYWwvcmVwb3J0","display":"internal/report"}
```

`display` is the exact UTF-8 string when the bytes are valid UTF-8 and otherwise
is `null`. Consumers compare `bytes_b64url`, never `display`. Text output
escapes control bytes and terminal escape characters.

Candidate IDs are SHA-256 over NUL-joined UTF-8 fields with no trailing NUL:

```text
tacita-candidate-v1, origin, antecedent bytes_b64url, consequent bytes_b64url
```

Profile candidate IDs replace antecedent and consequent with tier and rule ID.

Worked vector:

```text
antecedent bytes: config
antecedent bytes_b64url: Y29uZmln
consequent bytes: charts/kapparmor
consequent bytes_b64url: Y2hhcnRzL2thcHBhcm1vcg
candidate ID:
dd2fbb9a369995a25ef008e95a8b6027ebc4b72f44acd0689d933600f1150dbb
```

## `tacita.report/v1`

Top-level fields appear in this order:

| Field | Type | Contract |
| --- | --- | --- |
| `schema` | string | Always `tacita.report/v1` |
| `kind` | string | `calibration`, `backtest`, or `profile` |
| `status` | string | `complete`, `failed`, or `inconclusive` |
| `tool` | object | Tacita, Git, and optional Go helper versions |
| `repository` | object | Object format, root, resolved tip, first-parent counts |
| `configurations` | array | Ordered parameter sets and optional lock digest |
| `budgets` | array | Sorted deterministic limits and observed work counts |
| `partitions` | array | `60`, `70`, `80`, `90` cutoff and window records |
| `exclusions` | array | Reason/count pairs sorted by reason |
| `candidates` | array | Origin-specific candidate records |
| `baselines` | array | Frequent, unweighted, shuffled, and no-history records |
| `controls` | array | Young/template control outcomes |
| `profiles` | array | Tier/rule applicability and provenance |
| `preregistered_checks` | array | Positive-control metrics and eligibility |
| `metrics` | array | Per-configuration repository metrics and diagnostics |
| `gates` | array | Deterministic dimensions sorted by gate ID |
| `limitations` | array | Sorted stable limitation codes |
| `failure` | object or null | Typed failure code, bounded detail, and limit data |

`status` describes report production, not experiment acceptance. A complete
report may contain failed gates.

### Repository and partition records

`tool` fields are `tacita_version`, `tacita_binary_sha256`, `git_version`, and
`go_helper_version`, in that order. The optional helper version is `null` when
no profile helper ran.

`repository` fields are `object_format`, `root_commit`, `resolved_commit`,
`reachable_commits`, `first_parent_events`, `single_parent_events`, and
`merge_result_events`, in that order.

Each `configurations` record contains `configuration_id`,
`development_lock_digest`, `size_weight`, `minimum_opportunities`,
`minimum_confidence`, and `minimum_lift`, in that order. Calibration reports
contain all 81 records in configuration-ID order; selected backtests contain
one; profile-only reports contain an empty array.

Each budget record contains `id`, `unit`, `limit`, `observed`, and `outcome`.
Runtime and memory budget records contain limits but set canonical `observed`
and `outcome` to `null`; their observations live in the operational sidecar.

Each exclusion record contains `scope`, `reason`, and `count`. Scope is `event`
or `path`. Records are sorted by scope then reason.

Partition records are ordered `60`, `70`, `80`, `90` and contain percentile,
cutoff object ID, inclusive training event indices, inclusive evaluation event
indices, eligible counts, displayed candidate count, and deterministic work
counts. Indices are one-based.

### Candidate records

Candidates are ordered by configuration ID, partition, rank, then candidate ID.
A final full-history proposal uses partition `100`.

Each inferred candidate contains:

1. `id`, `origin`, `configuration_id`, `partition`, and `rank`;
2. antecedent and consequent byte identities;
3. raw opportunity and support integers;
4. weighted exposure, support, confidence, prevalence, and lift decimal
   strings;
5. bounded supporting and exception evidence arrays ordered by integration
   event index and object ID.

Evidence contains event object ID, event kind, one-based integration index, and
sorted path/status records. Canonical reports never contain blob contents.

Each candidate retains at most 20 supporting and 20 exception events. When
more exist, retain the 10 earliest and 10 latest by integration index, then
render them in ascending index/object-ID order. Within one evidence event,
retain every path mapped to the candidate's antecedent or consequent plus at
most 20 other paths selected by path-byte order. Report total pre-sampling
event and path counts beside the bounded evidence.

Calibration and control reports set candidate evidence arrays empty while
retaining total evidence counts. Full evidence is generated only for the
selected configuration's development diagnostics and holdout backtest.

Profile records use the same ID and provenance fields defined in
[`profiles.md`](profiles.md), plus applicability, compliance evidence, and
optional non-applicability reason.

Each preregistered check contains `id`, antecedent and consequent byte
identities, raw opportunity, raw support, weighted confidence, weighted lift,
`eligible`, and `excluded_by`, in that order. `excluded_by` lists every
selected-configuration threshold that rejected the pair. The check is reported
even when the pair is not an eligible or displayed candidate.

### Metrics and gates

Each repository metric record contains `configuration_id`,
`micro_adherence`, `macro_adherence`, `coverage`, `cutoff_abstention`,
`minimum_jaccard`, and `rank_correlation_diagnostics`, in that order. Baseline
and control records contain `configuration_id`, `id`, the same applicable
metric fields, and diagnostic outcome. Baseline and control outcomes do not
enter the conjunctive engineering gate unless a frozen requirement names that
specific baseline or control.

A metric is:

```json
{"value":"0.812500","reason":null}
```

or:

```json
{"value":null,"reason":"no_opportunities"}
```

Gate records contain `configuration_id`, `id`, `outcome`, `metric`, `operator`,
`threshold`, and `reason`, in that order. Profile-wide gates use `null`
configuration ID. Outcomes are `pass`, `fail`, or `inconclusive`.

Stable undefined-metric reasons are:

```text
no_eligible_evaluation_events
no_opportunities
no_candidates
below_inference_floor
identical_baseline
empty_candidate_sets
insufficient_rank_overlap
profile_not_applicable
```

Stable failure codes are:

```text
unsupported_platform
unsupported_architecture
unsupported_git_version
unsupported_object_format
incomplete_repository
malformed_git_output
malformed_profile_input
profile_helper_failure
budget_events
budget_unique_paths
budget_unique_components
budget_pair_observations
budget_unique_pairs
budget_git_stdout
budget_git_stderr
budget_elapsed
budget_memory
budget_report_rows
budget_report_bytes
cancelled
internal_invariant
```

Stable exclusion reasons are:

```text
root_event
no_eligible_paths
event_path_limit
event_component_limit
vendor_path
gitlink_path
```

Stable limitation codes are:

```text
merge_policy_sensitive
pull_request_boundaries_unavailable
rename_identity_unavailable
generated_status_not_inferred
sha1_only
linux_amd64_only
profile_policy_opinion
human_inspected_holdout
single_reviewer
single_configuration_holdout
small_review_sample
self_attested_freeze
timing_label_coupling
evidence_metadata_confidential
```

`failure` fields are `code`, `operation`, `observed`, `limit`, and
`diagnostic_digest`. Raw Git stderr and local paths are operational diagnostics,
not canonical failure detail.

For `budget_elapsed` and `budget_memory`, canonical `observed` is `null` unless
Tacita's own accounting observed the limit before external termination.

For the 100,000-row budget, one row is one partition, exclusion, candidate,
candidate-evidence event, baseline, control, profile, preregistered check,
metric diagnostic, gate, limitation, or failure record. Path records retained
inside one candidate-evidence event do not add rows. Top-level scalar objects
do not add rows.

## `tacita.grid/v1`

The canonical grid fields are schema, minimum training transactions, minimum
raw support, ordered size weights, ordered opportunity thresholds, ordered
confidence thresholds, ordered lift thresholds, display limit, ordered ranking
keys, and grid digest. Values are exactly those frozen in the candidate
configuration grid. The digest is SHA-256 over the preceding canonical fields.

## `tacita.dev-lock/v1`

The canonical development lock fields are:

1. `schema`: `tacita.dev-lock/v1`;
2. `corpus`: ordered development and control repository object IDs;
3. `environment`: Linux architecture, exact Git version, Tacita binary digest,
   and optional Go helper version;
4. `grid_digest`: SHA-256 of canonical grid JSON;
5. `selector`: `development-selector-v1`;
6. `configuration_id`;
7. `parameters`: size weight, opportunity, confidence, and lift thresholds;
8. `development_reports`: ordered repository/report-digest pairs;
9. `selector_metrics`: ordered metric/value pairs;
10. `lock_digest`: SHA-256 of the preceding canonical fields.

The holdout command requires the lock and verifies every embedded digest and
repository object ID before reading holdout history.

## `tacita.review-manifest/v1`

The review manifest contains schema, report digest, repository object ID,
selected candidate IDs, selected deviation identities, evidence references,
selection hashes, and manifest digest. It is written before candidate content
is displayed. Labels and decision durations are written to a separate review
result that references the immutable manifest digest. The manifest digest is
SHA-256 over the preceding canonical fields.

## `tacita.review-result/v1`

The review result contains schema, status (`complete` or `invalid`), manifest
digest, reviewer relationship, ordered candidate and deviation labels, evidence
references inspected, optional rationale, monotonic elapsed decision durations,
optional invalidation reason, and result digest. The frozen invalidation reason
is `external_interruption`. The result digest is SHA-256 over the preceding
canonical fields. The result never modifies the immutable analytical report or
review manifest.

## `tacita.evidence/v1`

The evidence index is the final experiment-run envelope. It contains:

1. schema and experiment ID;
2. ordered artifact kind/digest pairs for the development lock, analytical
   reports, review manifest, review result, and operational sidecars;
3. deterministic analytical gate outcomes;
4. operational time and memory gate outcomes;
5. product-review gate outcomes;
6. final lane and experiment outcomes;
7. evidence-index digest.

The evidence-index digest is SHA-256 over the preceding canonical fields.
The index is canonical for a recorded run, but two executions may have
different sidecar digests and therefore different evidence-index bytes. The
byte-identity engineering gate applies to analytical reports generated with
identical resolved inputs, configurations, and tool versions.

## `tacita.operation/v1`

The operational sidecar is not deterministic experiment evidence. It is
serialized with the canonical JSON rules above and contains these fields in
order:

1. `schema`: `tacita.operation/v1`;
2. `report_digest`;
3. `elapsed_duration_ns`;
4. `peak_resident_bytes`;
5. `process_exit_status`;
6. `reference_runner`;
7. `diagnostic_paths`, which is `null` when absent;
8. `sidecar_digest`, SHA-256 over the preceding canonical fields.

The sidecar is size-bounded, must never contain source blobs or unredacted Git
stderr, and never rewrites canonical report bytes. It records the runtime and
memory gate outcomes used alongside the canonical analytical gates in the
experiment evidence bundle.

## Text rendering

Text is rendered exclusively from a decoded v1 model. It does not recompute
metrics or gates. Sections follow top-level report order; records retain
canonical array order. Arbitrary bytes and control characters are escaped.
