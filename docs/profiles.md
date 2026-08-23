# Profile Contract

This document owns the first experiment's profile identifiers, applicability,
parsing, and provenance. Product intent remains in
[`product.md`](product.md); adoption thresholds remain in
[`experiment.md`](experiment.md).

## Common behavior

Profiles are explicitly enabled by tier. Tacita evaluates files from the
resolved Git commit, never from an uncommitted worktree. Profile candidates:

- retain origin `profile`;
- include tier, rule ID, profile version, source URL, and source commit;
- report applicability and evidence independently of current compliance;
- never claim to be inferred from repository history;
- cannot compensate for a failed inferred-candidate lane.

The initial profile bundle version is `profiles/v1`. Rule identity is
`<tier>/<rule-id>`. Unknown tiers, rules, parser output, or profile versions are
typed errors rather than ignored input.

Profile evaluation reads at most 32 `go.mod` files of at most 1 MiB each and
100,000 `.go` path entries. Vendor paths and gitlinks follow the architecture
exclusions. Budget exhaustion makes the profile lane non-passing.

## Go metadata parser

The `go-core` tier requires the frozen Go 1.27.0 toolchain as an optional
profile helper; inferred mining still requires only Git. For each `go.mod` blob:

1. write the exact bounded blob bytes to a private temporary file;
2. run `go mod edit -json <file>` with `GOTOOLCHAIN=local`, `GOWORK=off`,
   `GOPROXY=off`, and `GOSUMDB=off`;
3. bound stdout, stderr, and elapsed time under the profile budgets;
4. decode the documented JSON fields needed below;
5. remove the temporary file on every success, error, or cancellation path.

The command is parse-only and must not run module loading, download
dependencies, or modify repository files. Missing Go 1.27.0, malformed `go.mod`,
unexpected JSON, or a helper failure is explicit and non-passing.

Module roots are the repository-relative parent directories of `go.mod`; the
root module uses `.`. Roots and module paths are byte-sorted. Duplicate module
paths are invalid profile input.

## `go-core`

Provenance:

- source: [official Go module documentation](https://go.dev/ref/mod);
- profile version: `go-core/v1`;
- parser toolchain: Go 1.27.0.

| Rule ID | Applicability | Proposed expectation | Evidence |
| --- | --- | --- | --- |
| `module-topology` | At least one valid `go.mod` | Preserve the exact set of module roots and declared module paths unless a maintainer edits the ratified rule | Sorted root/path pairs |
| `go-version-floor` | At least one valid `go.mod` | Every module declares `go 1.25` or newer | Module roots and parsed `go` directives |
| `no-local-replace` | At least one valid `go.mod` | No `replace` target is a local filesystem path without a module version | Module root and parsed replacement entries |
| `semantic-import-version` | At least one valid `go.mod` | Module path suffixes use `/vN` with `N >= 2`, except documented `gopkg.in` `.vN` paths | Module roots and declared module paths |

`go-version-floor` and `no-local-replace` are pinned profile opinions for
`profiles/v1`, not claims that older directives or local development replaces
are invalid Go. `semantic-import-version` accepts the documented `gopkg.in`
`.vN` form and otherwise checks `/vN`; release-tag compatibility remains
outside the initial offline diff contract.

A replacement is local when the parsed replacement has an empty version; no
path-prefix heuristic is used.

## `spf13-idiomatic`

Provenance:

- source:
  [`spf13/go-skills/go/SKILL.md`](https://github.com/spf13/go-skills/blob/e67851cfcca008592c7c4965b8220c7cb37e2f1c/go/SKILL.md);
- source commit: `e67851cfcca008592c7c4965b8220c7cb37e2f1c`;
- profile version: `spf13-idiomatic/v1`.

A Go package directory is a non-vendor directory containing at least one
non-test `.go` path at the resolved commit. Its nearest module root is the
longest path-prefix module root. Tacita does not parse build tags or Go source
contents for these rules.

| Rule ID | Applicability | Proposed expectation | Evidence |
| --- | --- | --- | --- |
| `max-package-depth` | At least one Go package directory | Package directories are at most one segment below their nearest module root | Sorted violating package directories and depths |
| `domain-over-layers` | At least one Go package directory | No package directory basename is `service`, `repository`, `controller`, or `domain` | Sorted matching directories |
| `no-generic-packages` | At least one Go package directory | No package directory basename is `utils`, `helpers`, or `common` | Sorted matching directories |
| `no-heavy-test-frameworks` | At least one valid `go.mod` | Module requirements exclude Ginkgo and mock-generation frameworks | Module roots and matching requirements |

The frozen heavy-framework module prefixes are:

```text
github.com/onsi/ginkgo
github.com/golang/mock
go.uber.org/mock
```

Matching is exact or prefix followed by `/`; plain string-prefix matches do not
count.

The depth, directory-name, and module-prefix parameters are Tacita-authored
operationalizations of the pinned source's broader guidance. They are not
literal identifiers supplied by the upstream document.

The earlier provisional `no-root-pkg` rule is removed: the pinned source
explicitly recommends a single root package as the default for simple
applications. Profile rules may be opinionated, but they must not invert their
declared provenance.

## Applicability and candidate output

Each enabled, applicable rule emits exactly one candidate even when current
evidence complies. Non-applicable rules emit no candidate and record a reason.
Current violations are evidence, not automatic rejection.

Profile candidates are ordered by enabled tier order (`go-core`,
`spf13-idiomatic`) and then rule ID bytes. Human-review presentation applies the
frozen hash order from the product gate. The Proficiency adoption run enables
both tiers and requires at least three applicable candidates before timing can
pass.
