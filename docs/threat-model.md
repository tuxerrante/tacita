# Threat Model

This threat model covers the offline evidence-first experiment and its optional
Go profile helper. It does not claim that Tacita is a sandbox.

## Assets

- integrity and determinism of candidate, metric, and gate results;
- availability of CPU, memory, disk, and child-process capacity;
- confidentiality of unrelated local files and environment values;
- integrity of the analyzed repository and Git object database;
- integrity of the development configuration lock and review manifest;
- terminal safety of text output.

## Trust boundaries

Untrusted inputs are repository paths, revision strings, Git objects and refs,
path bytes, history shape, repository-local configuration, profile input blobs,
configuration files, and decoded report files.

Trusted dependencies for the first experiment are the Tacita binary, Linux
kernel and filesystem, Git 2.43 or newer, the frozen optional Go 1.26.5 profile
helper, the pinned profile source revisions, and the external measurement
harness. Compromise of those dependencies is outside scope.

## Threats and controls

| Threat | Example | Required controls | Residual risk |
| --- | --- | --- | --- |
| Argument or option injection | Revision begins with `-` | Argument arrays, `--end-of-options`, full object-ID resolution before traversal | Bugs in Git itself |
| Shell execution | Repository path contains shell syntax | Invoke trusted local Git directly with argument arrays; never construct a shell command | Compromised executable |
| Ambient Git behavior | Global aliases, inherited `GIT_DIR`, tracing, pager, external diff | Allowlisted environment; disable system/global config, prompts, tracing, pager, external diff, text conversion, and optional locks | Repository parsing still uses Git |
| History substitution | Replace refs, grafts, alternates, missing promisor objects | Disable replacements; reject grafts, alternates, promisors, and missing objects; report root/tip | Undetected Git implementation defects |
| Hook or filter execution | Malicious checkout filter | Use object and diff commands that do not checkout; never invoke hooks | Future commands must preserve this invariant |
| Parser confusion | Newlines, tabs, leading dashes, non-UTF-8 paths | NUL framing, byte-oriented decoding, explicit change-status grammar, fuzzing | Git output grammar changes |
| Terminal injection | Path contains escape sequences | Escape control bytes in text; canonical JSON stores byte identities safely | A consumer may render JSON unsafely |
| Path traversal | Symlink points outside repository | Never follow symlinks; use private temporary files only for bounded profile blobs | Local filesystem race outside Tacita |
| Resource exhaustion | Huge merge, path fan-out, pair explosion | Per-event exclusions, global event/path/component/pair/byte/time/memory limits | Work below limits may still be expensive |
| Silent partial success | Timeout after some candidates | Typed non-passing failure; no truncated success report | Operator may ignore failure status |
| Nondeterministic output | Map order, clock, locale, floating-point edge | Stable ordering, fixed locale/timezone, finite metrics, canonical formatting, operational sidecar | Different supported tool versions may differ |
| Concurrent repository mutation | Object removed during traversal | Resolve immutable object ID; fail on missing or changed required objects | No repository-wide lock is taken |
| Local data disclosure | Report captures source or environment | Evidence uses paths, statuses, and object IDs; no source blobs in reports; bounded redacted stderr | Paths and commit metadata may themselves be sensitive |
| Profile-helper network access | Go attempts toolchain or module download | `GOTOOLCHAIN=local`, `GOWORK=off`, `GOPROXY=off`, `GOSUMDB=off`; parse-only command | Compromised local Go tool |
| Profile provenance confusion | Opinion presented as official Go behavior | Tier and rule provenance in every candidate; pinned source commit | Maintainer may still accept unsuitable guidance |
| Holdout contamination | Calibration reads Kapparmor metrics | Configuration lock required before Tacita holdout reports; report digests and corpus IDs recorded | Humans have inspected public Kapparmor history; only configuration independence is claimed |
| Report or lock substitution | Different config used for holdout | Canonical bytes and SHA-256 digests; resolved IDs embedded in reports | SHA-256 implementation compromise |

## Explicit exclusions

The first experiment does not:

- protect against a compromised OS, filesystem, Git, Go toolchain, or Tacita
  binary;
- isolate arbitrary code execution in a sandbox;
- clone repositories or authenticate to remote providers;
- recover pull-request boundaries from forge APIs or commit messages;
- read or execute repository build scripts, hooks, tests, binaries, or
  generated tools;
- infer dataflow, secrets, vulnerabilities, authorship, or intent from source
  contents;
- guarantee confidentiality for repository paths and commit metadata included
  as evidence;
- provide ratification or enforcement security, which belongs to a later
  post-experiment threat model.

## Security validation

Implementation must include real temporary-repository integration tests,
fuzzing of NUL/status/path/report decoders, cancellation tests that await child
processes, repeated canonical-byte tests, race tests, reachable-vulnerability
analysis, and secret scanning. Budget and unsupported-environment failures are
tested as first-class outcomes.
