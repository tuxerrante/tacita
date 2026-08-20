# Agent Instructions

## Repository state

Tacita is an experimental Go 1.26 CLI for turning repository-local conventions
into explicit, human-ratified contracts. The checked-in executable is only a
disposable `backtest` shell. It does not read Git history, mine candidates,
evaluate profiles, ratify rules, or enforce policy.

Milestone 0 is complete, amended, and re-frozen. Implement only the active
milestone under the frozen experiment contracts. Do not change corpus IDs,
thresholds, report fields, platform guarantees, resource budgets, or
Git/component semantics from development or holdout results.

## Load context by task

Always read:

1. [`README.md`](README.md);
2. [`IMPLEMENTATION_PLAN.md`](IMPLEMENTATION_PLAN.md).

Then load only the relevant owned document:

- product behavior or scope: [`docs/product.md`](docs/product.md);
- mining, metrics, corpus, reports, or acceptance:
  [`docs/experiment.md`](docs/experiment.md);
- Go, CLI, Git, security, packages, errors, or tests:
  [`docs/architecture.md`](docs/architecture.md);
- profile parsing or rules: [`docs/profiles.md`](docs/profiles.md);
- report artifacts or encoding: [`docs/report-v1.md`](docs/report-v1.md);
- threats, controls, or exclusions: [`docs/threat-model.md`](docs/threat-model.md);
- repository workflow: [`CONTRIBUTING.md`](CONTRIBUTING.md).

[`docs/README.md`](docs/README.md) explains document ownership. When a decision
changes, edit its owning document and link to it instead of copying it.

## Authority

Explicit repository decisions and the owning documents above take precedence.
For Go behavior, use the selected toolchain and current official Go
documentation before generic guidance. This file summarizes operating
constraints; [`docs/architecture.md`](docs/architecture.md) owns their detailed
contract and wins if this summary drifts.

## Commands

Requirements are Go 1.26, Git, GNU Make, and `uv`.

```bash
make fmt           # format Go source and Markdown
make markdown-check # lint Markdown
make test          # unit and integration tests
make lint-new      # lint changes since LINT_BASE
make check         # fast local gate
make quality-gate  # full pre-publication gate
```

Pinned tools are installed under `.go/bin` and are not runtime dependencies.
Run focused tests while iterating:

```bash
go test ./cmd/tacita -run '^TestRun$' -count=1
```

## Implementation invariants

- Keep the core offline, provider-neutral, read-only, and initially free of
  runtime module dependencies.
- Continue using the standard library until measured complexity justifies a
  dependency.
- Treat repositories as untrusted. Invoke Git with `exec.CommandContext` and
  argument arrays, never a shell.
- Resolve revisions with `rev-parse --verify --end-of-options`, validate the
  full object ID, use NUL-delimited output, isolate ambient Git configuration,
  and cap all work and output.
- Support the frozen Linux/amd64, Git 2.43+, SHA-1 boundary and model
  first-parent integration events without rename inference.
- Return typed/wrapped errors and print each error once at the CLI boundary.
- Keep canonical reports deterministic and render JSON/text from one model.
- Preserve the distinction between `inferred` and `profile` candidate origins.
- Nothing is enforceable before explicit human ratification.
- Prefer concrete types and consumer-owned interfaces. Keep packages shallow
  and domain-named; do not add `pkg`, `utils`, or architectural layers.
- Keep execution sequential until profiling shows a bottleneck.
- Test the Git boundary with real temporary repositories, not mocks.

## Change rules

- Make precise changes and preserve unrelated local work.
- Work on a task branch and deliver versioned changes through a pull request;
  do not commit directly to `main`.
- Give every pull request exactly one independent motivation. Sharing a
  subsystem, milestone, reviewer, or implementation boundary is not sufficient.
- Before implementation, classify every planned file by motivation. If any file
  needs a different rationale, stop and split the work into separate or stacked
  pull requests.
- Documentation may accompany code only when it documents that exact behavior,
  API, or operational requirement. Independent clarification belongs in its own
  pull request.
- Run `/review-pr-hygiene` against the final diff before publishing or updating
  a pull request. Treat mixed concerns as blocking, not advisory. Use squash as
  the merge method.
- Explicitly request Copilot review on every pull request. Resolve every review
  conversation before evaluating merge readiness.
- Write committed documentation, comments, and user-facing text in English.
- Personal Italian coaching belongs only in ignored local files.
- Do not commit generated `bin/`, `.go/`, or `coverage.out` artifacts.
- Do not publish, create remotes, open pull requests, or rewrite shared history
  without explicit approval.
