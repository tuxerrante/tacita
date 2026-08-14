# Tacita Documentation

The documentation is organized by question so readers and coding agents can
load only the context they need.

## Reading paths

### Understand the product

1. [Product](product.md)
2. [Implementation plan](../IMPLEMENTATION_PLAN.md)

This path explains the user problem, workflow, current state, and next decision
without requiring statistical or implementation detail.

### Work on the experiment

1. [Product](product.md)
2. [Experiment](experiment.md)
3. [Implementation plan](../IMPLEMENTATION_PLAN.md)

Use this path when changing mining semantics, weighting, baselines, corpus
selection, report evidence, or go/no-go thresholds.

### Work on Go code

1. [Implementation plan](../IMPLEMENTATION_PLAN.md)
2. [Architecture](architecture.md)
3. [Contributing](../CONTRIBUTING.md)

Use this path for CLI, Git ingestion, packages, errors, tests, security, or
performance work.

### Prepare an agent

Start at [`AGENTS.md`](../AGENTS.md). It contains the repository state,
authority order, validation commands, and context-loading rules. Client-specific
instruction files should remain thin adapters to that file.

## Document ownership

| Document | Owns | Does not own |
| --- | --- | --- |
| [`README.md`](../README.md) | Public entry point and quick start | Detailed design |
| [`product.md`](product.md) | Product claim, user loop, boundaries | Implementation sequence |
| [`IMPLEMENTATION_PLAN.md`](../IMPLEMENTATION_PLAN.md) | Active status, blockers, milestones | Detailed protocol or architecture |
| [`experiment.md`](experiment.md) | Hypotheses, metrics, evidence, stop rules | Package design |
| [`architecture.md`](architecture.md) | Technical, security, and testing contracts | Product acceptance thresholds |
| [`AGENTS.md`](../AGENTS.md) | Agent operating context | Product or architecture rationale |
| [`CONTRIBUTING.md`](../CONTRIBUTING.md) | Human contribution workflow | Product specification |

When a decision changes, edit the document that owns it and link to that
decision elsewhere. Avoid parallel summaries that can drift.

Committed project documentation is English. Ignored personal coaching notes may
be Italian.
