# Tacita Product Brief

## Product statement

Tacita discovers repository-local change conventions, asks maintainers to
ratify the ones that reflect intentional decisions, and reports when a later
diff deviates from those versioned contracts.

> Does this change deviate from how this repository intentionally works?

Tacita is not an AI detector and does not recommend the next feature or file to
edit. It turns tacit project knowledge into explicit, reviewable policy.

## Name

**Tacita** comes from the Latin root for silent or unspoken. It reflects the
product's purpose: make tacit repository rules explicit.

## Problem

Linters and tests handle universal rules well. They rarely encode local
decisions such as:

- which components normally change together;
- which repository boundaries are intentional;
- which exceptions are accepted and why;
- which conventions should apply to future changes.

Writing these policies manually has high setup cost. A senior maintainer who
can author every rule may choose to review every change instead.

Tacita reduces that bootstrap cost:

1. derive candidate expectations from repository history or an explicitly
   selected profile;
2. show evidence and exceptions;
3. ask a maintainer whether each candidate reflects intent;
4. store only accepted rules as repository-owned contracts;
5. evaluate explicit diffs locally or in any CI.

The initial adoption target is at least three useful ratifications within ten
minutes on an eligible repository when reviewed by a maintainer with direct
decision history. It is a pilot workflow measure, not an estimate of agreement
among maintainers.

## Candidate origins

A **candidate** is a reviewable proposed expectation. It is not policy and
cannot produce findings unless a maintainer later ratifies it as a rule.

Tacita keeps origin visible throughout proposal, ratification, and checking.

### Inferred

For mature repositories, **history mining** examines past file changes and
looks for repeated directional co-change between packages or directories.
Files and commits remain evidence; component relationships become inferred
candidates.

Historical regularity is not proof of architecture or causality.

### Profile

New repositories do not have enough history. A **profile** is an explicitly
selected, versioned catalog of prewritten candidate expectations. Repositories
can opt into:

- `go-core`: deterministic module and release contracts grounded in official
  Go behavior;
- `spf13-idiomatic`: clearly labelled, opinionated conventions selected from a
  pinned `spf13/go-skills` revision.

Profile guidance is never described as repository-inferred. A successful
profile does not compensate for a failed history-mining hypothesis.

## Product loop

The product review workflow shows candidate evidence to a maintainer, who can
accept, reject, or defer each proposal:

```text
tacita propose
  -> candidate evidence
  -> tacita ratify
  -> tacita.yml
  -> tacita check --base <revision> --head <revision>
  -> deterministic findings
```

`tacita.proposed.yml` is never enforced. `tacita.yml` is human-owned and is the
only policy source.

Newly ratified rules default to `warn`. Promotion to `block` is explicit for
each rule and never happens automatically.

## Initial experiment

The first implementation is an evidence-first experiment, not an enforcement
release:

```text
Git history
  -> normalized file evidence
  -> package/directory transactions
  -> directional candidates
  -> temporal and product-relevance evaluation
  -> deterministic reports
```

The experiment must show both:

1. candidate regularity survives chronological holdout and beats trivial
   baselines;
2. maintainers recognize candidates as intentional decisions and sampled
   deviations as review-worthy.

If either condition fails, Tacita must stop or change the hypothesis rather
than lower thresholds.

## Operating principles

- Offline and provider-neutral core.
- Explicit revisions and read-only repository access.
- No shell command construction.
- Standard library first and zero runtime module dependencies initially.
- Deterministic, versioned JSON with text rendered from the same model.
- Sequential implementation before profiling justifies concurrency.
- Precision over recall; abstention is a valid result.
- Evidence and exceptions instead of opaque scores.
- No rule is enforced before human ratification.

## Non-goals

- attributing code to a person or AI agent;
- treating hooks as a security boundary;
- dataflow, taint, symbolic execution, or runtime invariant inference;
- deep learning or opaque model scoring;
- extracting executable policy from generated prose;
- network or LLM dependencies in the core analysis path;
- GitHub-specific behavior in the core CLI;
- Kubernetes or OCI packaging before a validated product need.
