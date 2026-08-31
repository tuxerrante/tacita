# Tacita

> Make tacit repository rules explicit.

Tacita is an experimental Go CLI that discovers repository-local change
conventions, asks maintainers to ratify the ones that reflect intentional
decisions, and reports when a later diff deviates from those versioned
contracts.

Tacita is not an AI detector and does not recommend the next feature or file to
edit.

## 🚦 Status

Milestone 0 is complete: the first experiment contract is amended and
re-frozen. Milestone 1 safe Git ingestion is implemented: the internal boundary
validates the supported environment, resolves and traverses first-parent
history, normalizes component transactions, and enforces its frozen event and
identity budgets. Milestone 2 descriptive mining is implemented: an internal
boundary accumulates directional pair state from component transactions and
derives ranked candidate metrics, supporting any of the 81 frozen grid
configurations. See [`IMPLEMENTATION_PLAN.md`](IMPLEMENTATION_PLAN.md) for the
exact status. The current binary remains only a testable CLI shell; it does not
analyze Git history or enforce policy. There are no releases yet.

## 🧭 Start here

Read only what matches the task:

1. [`docs/product.md`](docs/product.md) — what Tacita is, who it serves, and
   what it deliberately does not do.
2. [`IMPLEMENTATION_PLAN.md`](IMPLEMENTATION_PLAN.md) — current milestone,
   blocked decisions, and implementation order.
3. [`docs/experiment.md`](docs/experiment.md) — hypotheses, evaluation protocol,
   metrics, and stop conditions.
4. [`docs/architecture.md`](docs/architecture.md) — technical reference for
   CLI, Git, packages, security, and tests; read the product brief first.

[`docs/README.md`](docs/README.md) is the complete documentation map.
Contributors should also read [`CONTRIBUTING.md`](CONTRIBUTING.md). Coding
agents start with [`AGENTS.md`](AGENTS.md), which points to the minimum context
needed for each kind of change.

## 🛠️ Development

Requirements:

- Go 1.27;
- Git;
- GNU Make;
- `uv`.

```bash
make help
make markdown-gate
make check
make quality-gate
```

`make check` runs the fast incremental developer gate. `make quality-gate`
runs the full lint, race, coverage, build, vulnerability/secret scanning,
Go/Markdown formatting, and Markdown lint checks. Pinned external development
tools are installed under `.go/bin`; they do not become runtime module
dependencies. A pull request whose changed files are all Markdown uses
`make markdown-gate` instead, which runs only the pinned Markdown formatter
check and linter.

Run the current shell:

```bash
go run ./cmd/tacita backtest --revision HEAD .
```

## 🤝 Contributing

Read [`CONTRIBUTING.md`](CONTRIBUTING.md) and
[`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) before contributing.

## 🔐 Security

Tacita treats analyzed repositories as untrusted input. See
[`SECURITY.md`](SECURITY.md) for the reporting process and current support
status.

## 📄 License

Licensed under the Apache License 2.0. See [`LICENSE`](LICENSE).
