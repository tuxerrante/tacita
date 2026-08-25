# Contributing to Tacita

Tacita is experimental. Discuss changes that alter the product claim, report
schema, Git semantics, security boundary, or statistical protocol before
implementation.

Start with the task-oriented documentation map in
[`docs/README.md`](docs/README.md).

## 🛠️ Development setup

Requirements:

- Go 1.27;
- Git;
- GNU Make;
- `uv`;
- `pre-commit` 4.x for the Git hooks.

```bash
make tools
make pre-commit-install
make check
```

Development tools are pinned and installed under `.go/bin`. The Go tools
(`gofumpt`, `gitleaks`, `golangci-lint`, `govulncheck`) are pinned with
[bingo](https://github.com/bwplotka/bingo): each
version lives in its own module under `.bingo/`, and `make tools` rebuilds a tool
whenever its `.bingo/<tool>.mod` changes, so a version bump takes effect without
`make clean`. `rumdl` is pinned by `RUMDL_VERSION` in the `Makefile` and
installed with `uv`. To change a Go tool version, build the pinned bingo, then
use it to rewrite the pin and commit the updated `.bingo/` files:

```bash
make .go/bin/bingo
GOBIN="$PWD/.go/bin" .go/bin/bingo get <tool>@<version>
```

`make .go/bin/bingo` relinks a bare `bingo` to the pinned versioned build, so the
command above never hard-codes the bingo version.

## 🔍 Validation

Use the smallest relevant target while iterating:

```bash
make fmt
make markdown-check
make test
make lint-new
make check
```

Before requesting review, run exactly one gate:

```bash
make markdown-gate # when every changed file is Markdown
make quality-gate  # for every mixed or non-Markdown change
```

`make check` uses incremental linting when a Git baseline exists.
`make markdown-gate` checks only Markdown formatting and lint.
`make quality-gate` always performs the full validation suite.

Neither gate runs the fuzzing engine, because a mutation search is
non-deterministic and cannot decide whether a change may merge. A scheduled
workflow searches for new inputs instead, and `make fuzz FUZZTIME=2m`
reproduces it locally. Commit any input it reports as a seed under
`testdata/fuzz`, so the deterministic gate covers it from then on.

## 🔀 Pull requests

All versioned changes go through a pull request. Keep each PR to one logical,
independently reviewable change; do not mix behavior, refactoring, formatting,
or unrelated documentation.

There is deliberately no line or file-count limit because generated changes
and small cross-cutting contracts make those counts poor proxies for review
cost. If a request spans multiple concerns or cannot be reviewed confidently
as one unit, split it into dependency-safe PRs. Stacked PRs are acceptable when
each layer builds and passes its own gate.

Before requesting review:

1. run `make markdown-gate` for a Markdown-only diff, or
   `make quality-gate` for every other diff;
2. complete every section of the pull request template, including side
   effects, test levels, the strongest opposing argument, residual risk, and
   any human decision the reviewer must make.

Squash is the only supported merge method. The PR title becomes the durable
change summary, so use a focused conventional-commit title. Delete the source
branch after merge.

## 🐹 Go conventions

- Prefer the standard library and concrete types.
- Define interfaces at the consumer only when substitution is real.
- Pass `context.Context` as the first argument to blocking operations.
- Return errors with operation context and preserve identities with `%w`.
- Print each error once at the CLI boundary.
- Use table-driven tests and real temporary Git repositories at integration
  boundaries.
- Do not add concurrency before a sequential benchmark demonstrates a need.
- Do not add runtime dependencies without a reviewed justification.

## 📚 Documentation

Committed documentation, architecture notes, code comments, and user-facing
text are written in English. Personal coaching notes must remain in ignored
local files.

Use the ownership table in [`docs/README.md`](docs/README.md) before changing a
decision. Update the owning document, then replace repeated detail elsewhere
with a link instead of maintaining parallel summaries. `make fmt` formats both
Go and Markdown; `make markdown-check` applies the Markdown rules.

### Emoji vocabulary

Markdown headings carry an emoji so a reader can locate a purpose without
reading prose. The value comes from consistency, so the same purpose always
uses the same emoji.

| Emoji | Purpose |
| --- | --- |
| 🧭 | Orientation: where to start, reading paths |
| 🚦 | Status: current state, milestone, blockers |
| 🛠️ | Setup, tooling, development environment |
| 🔍 | Validation, verification, recorded evidence |
| 🧪 | Tests and test levels |
| 🔀 | Pull requests and merges |
| 🧾 | Commits and history |
| 🐹 | Go code and language conventions |
| 📚 | Documentation, its map, and its ownership |
| 🏗️ | Architecture and package design |
| 📊 | Experiment, metrics, corpus, thresholds |
| 🔐 | Security, threats, untrusted input |
| 🤝 | Contributing and conduct |
| 📄 | License and legal terms |
| 📋 | Checklist or template surface |
| 📝 | Summary or description |
| 🎯 | Motivation, scope, boundary |
| 💥 | Side effects and blast radius |
| 🥊 | Strongest opposing argument |
| ⚠️ | Risk, caution, residual exposure |
| 🙋 | Human decision required |
| ✅ | Covered, satisfied, applicable |
| ➖ | Not applicable |

Rules:

- put exactly one emoji, at the start of a `##` heading; leave document titles
  and deeper headings plain, except a checklist surface such as
  [`.github/pull_request_template.md`](.github/pull_request_template.md), whose
  title marks the surface itself;
- reuse ✅ and ➖ for status cells in tables;
- when a document needs a purpose the table omits, add the row in the same pull
  request instead of inventing a local symbol;
- emoji belong to documentation. Never emit them from Go source, program
  output, or commit titles.

A document adopts this vocabulary during its next substantive update. Rewriting
an untouched document only to add emoji is not a motivation for a pull request.

The installed pre-commit hook also checks common file errors, module tidiness,
and staged secrets with Gitleaks. Run the full hook set with:

```bash
make pre-commit-run
```

## 🧾 Commits

Use focused conventional commits such as:

- `feat:`
- `fix:`
- `test:`
- `docs:`
- `refactor:`
- `build:`
- `chore:`

Do not mix generated artifacts or unrelated formatting with behavioral changes.

## 📄 License

By contributing, you agree that your contributions are licensed under the
Apache License 2.0.
