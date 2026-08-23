# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 🧭 Authority and precedence

[`AGENTS.md`](AGENTS.md) is the canonical cross-client agent context and the
single source for repository state, operating constraints, commands, and change
rules. Read it and [`IMPLEMENTATION_PLAN.md`](IMPLEMENTATION_PLAN.md) before any
change. This file is a thin Claude-facing adapter: it adds no separate policy and
defers to `AGENTS.md`; if the two disagree at the same directory depth, flag the
inconsistency instead of guessing.

Owned documents win over any summary, so edit the document that owns a decision
instead of copying it here:

- [`docs/product.md`](docs/product.md) — scope and boundaries;
- [`docs/experiment.md`](docs/experiment.md) — mining, metrics, corpus, gates;
- [`docs/architecture.md`](docs/architecture.md) — CLI, Git, packages, security,
  tests;
- [`docs/profiles.md`](docs/profiles.md) — profile parsing and rules;
- [`docs/report-v1.md`](docs/report-v1.md) — report artifacts and encoding;
- [`docs/threat-model.md`](docs/threat-model.md) — threats, controls, exclusions.

[`docs/README.md`](docs/README.md) maps document ownership.

## 🤝 Working as Claude Code

- Load the minimum context for each kind of change from the task-based reading
  guide in [`AGENTS.md`](AGENTS.md) rather than re-deriving it.
- Run the validation commands documented in [`AGENTS.md`](AGENTS.md)
  (`make check`, `make quality-gate`, and focused `go test`); this file keeps no
  parallel command list.
- Follow the change rules in [`AGENTS.md`](AGENTS.md): one motivation per pull
  request, task branch only, and run `/review-pr-hygiene` on the final diff
  before publishing, treating mixed concerns as blocking.
- Write committed text in English and follow the documentation emoji vocabulary
  in [`CONTRIBUTING.md`](CONTRIBUTING.md): exactly one emoji at the start of each
  `##` heading, and never emit emoji from Go source, program output, or commit
  titles.
