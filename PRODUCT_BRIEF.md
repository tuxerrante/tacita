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

Working tagline:

> Make tacit repository rules explicit.

The previous working name, AWG (Agentic Workflow Guardrails), described an
abandoned product direction and is retained only in local history.

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

The adoption target is at least three useful ratifications within ten minutes
on an eligible repository.

## Candidate origins

Tacita keeps origin visible throughout proposal, ratification, and checking.

### Inferred

Mature repositories can produce directional package or directory co-change
candidates from Git history. Files and commits remain evidence; component
relationships are the proposed rule.

Historical regularity is not proof of architecture or causality. An inferred
candidate becomes policy only after a maintainer ratifies it.

### Profile

New repositories do not have enough history. They can explicitly opt into:

- `go-core`: deterministic module and release contracts grounded in official
  Go behavior;
- `spf13-idiomatic`: clearly labelled, opinionated conventions selected from a
  pinned `spf13/go-skills` revision.

Profile guidance is never described as repository-inferred. A successful
profile does not compensate for a failed history-mining hypothesis.

## Product loop

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

## Current status

Tacita is in specification and learning-bootstrap stage. There is no usable
analysis, proposal, ratification, or enforcement implementation yet.

`IMPLEMENTATION_PLAN.md` defines the experiment, security model, package
boundaries, quality gates, and milestones.
