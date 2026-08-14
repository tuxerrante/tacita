# Tacita

> Make tacit repository rules explicit.

Tacita is an experimental Go CLI that discovers repository-local change
conventions, asks maintainers to ratify the ones that reflect intentional
decisions, and reports when a later diff deviates from those versioned
contracts.

Tacita is not an AI detector and does not recommend the next feature or file to
edit.

## Status

Tacita is in specification and learning-bootstrap stage. The current binary is
only a testable CLI shell; it does not analyze Git history or enforce policy.
There are no releases yet.

See:

- [`PRODUCT_BRIEF.md`](PRODUCT_BRIEF.md) for the product boundary;
- [`IMPLEMENTATION_PLAN.md`](IMPLEMENTATION_PLAN.md) for the preregistered
  experiment, security model, and milestones.

## Development

Requirements:

- Go 1.26;
- Git;
- GNU Make.

```bash
make help
make check
make quality-gate
```

`make check` runs the fast incremental developer gate. `make quality-gate`
runs the full lint, race, coverage, build, security, and formatting checks.
Pinned external development tools are installed under `.go/bin`; they do not
become runtime module dependencies.

Run the current shell:

```bash
go run ./cmd/tacita backtest --revision HEAD .
```

## Design principles

- Offline and provider-neutral core.
- Read-only Git access with explicit revisions.
- Standard library first.
- Deterministic reports and explicit resource budgets.
- Human ratification before enforcement.
- Precision over recall; abstention is valid.
- Sequential implementation until profiling justifies concurrency.

## Contributing

Read [`CONTRIBUTING.md`](CONTRIBUTING.md) and
[`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) before contributing.

## Security

Tacita treats analyzed repositories as untrusted input. See
[`SECURITY.md`](SECURITY.md) for the reporting process and current support
status.

## License

Licensed under the Apache License 2.0. See [`LICENSE`](LICENSE).
