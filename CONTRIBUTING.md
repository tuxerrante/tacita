# Contributing to Tacita

Tacita is experimental. Discuss changes that alter the product claim, report
schema, Git semantics, security boundary, or statistical protocol before
implementation.

Start with the task-oriented documentation map in
[`docs/README.md`](docs/README.md).

## Development setup

Requirements:

- Go 1.26;
- Git;
- GNU Make;
- `uv`;
- `pre-commit` 4.x for the Git hooks.

```bash
make tools
make pre-commit-install
make check
```

Development tools are pinned and installed under `.go/bin`.

## Validation

Use the smallest relevant target while iterating:

```bash
make fmt
make markdown-check
make test
make lint-new
make check
```

Before requesting review, run:

```bash
make quality-gate
```

`make check` uses incremental linting when a Git baseline exists.
`make quality-gate` always performs the full validation suite.

## Go conventions

- Prefer the standard library and concrete types.
- Define interfaces at the consumer only when substitution is real.
- Pass `context.Context` as the first argument to blocking operations.
- Return errors with operation context and preserve identities with `%w`.
- Print each error once at the CLI boundary.
- Use table-driven tests and real temporary Git repositories at integration
  boundaries.
- Do not add concurrency before a sequential benchmark demonstrates a need.
- Do not add runtime dependencies without a reviewed justification.

## Documentation

Committed documentation, architecture notes, code comments, and user-facing
text are written in English. Personal coaching notes must remain in ignored
local files.

Use the ownership table in [`docs/README.md`](docs/README.md) before changing a
decision. Update the owning document, then replace repeated detail elsewhere
with a link instead of maintaining parallel summaries. `make fmt` formats both
Go and Markdown; `make markdown-check` applies the Markdown rules.

The installed pre-commit hook also checks common file errors, module tidiness,
and staged secrets with Gitleaks. Run the full hook set with:

```bash
make pre-commit-run
```

## Commits

Use focused conventional commits such as:

- `feat:`
- `fix:`
- `test:`
- `docs:`
- `refactor:`
- `build:`
- `chore:`

Do not mix generated artifacts or unrelated formatting with behavioral changes.

## License

By contributing, you agree that your contributions are licensed under the
Apache License 2.0.
