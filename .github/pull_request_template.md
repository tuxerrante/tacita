# Pull request

<!--
These are the minimum checks for every pull request, human or agent authored.
This template is a checklist surface only. The owning documents decide the
rules: CONTRIBUTING.md for workflow, AGENTS.md for agent operating
constraints, and docs/architecture.md for implementation, security, and
testing contracts.
-->

## Summary

<!-- What changes, and why is this the smallest coherent PR? -->

## Motivation boundary

- [ ] This PR has exactly one independent motivation.
- [ ] Every file in the diff serves that motivation.
- [ ] Documentation here documents exactly this behavior, API, or requirement.
- [ ] This PR needs no split, or its dependency-safe stack is linked below.

<!-- Write "Not applicable" when this PR is not part of a stack. -->

## Side effects

<!--
State what this change touches beyond its own call path: files or packs
written, child processes started, filesystem and network access, environment
and Git configuration read, resource budgets consumed, output bytes, exit
codes, and CI behavior. Write "None" only when the diff genuinely has none.
-->

## Test levels

Record every level as covered, or as not applicable with the reason.
[`docs/architecture.md`](../docs/architecture.md#testing) owns the contract.

| Level | Status | Evidence |
| --- | --- | --- |
| Table-driven unit tests | | |
| Coverage at or above the gate | | |
| Race detector | | |
| Order independence with `-shuffle=on` | | |
| Fuzzing against a property oracle, not absence of panic alone | | |
| Committed fuzz seed for each new rule | | |
| Integration against real temporary repositories | | |
| Adversarial inputs: hostile paths, revisions, and history shapes | | |
| Budget, cancellation, and unsupported-environment failures | | |
| Determinism: repeated runs produce identical bytes | | |
| Smoke through the built binary against a realistic target | | |
| User-interaction simulation for a UI or service surface | | |
| Benchmark before adding concurrency or a dependency | | |

## Validation

- [ ] `make quality-gate`
- [ ] New tests fail against the unfixed code.

<!--
Record what ran, what passed, and what was deliberately skipped and why. Name
any claim that is reasoned rather than measured.
-->

## Strongest opposing argument

<!--
The best concrete case that this change is wrong, unnecessary, or should be
designed differently, plus the checks used to try to falsify it. End with
"stands", "weakened", or "changed".
-->

## Risks

<!-- State the main residual risk or write "None identified". -->

## Human decision needed

<!--
Anything the reviewer must ratify: an interpretation of a frozen contract, an
approval-gated action, or an unresolved trade-off. Write "None" when the
change decides nothing on the reviewer's behalf.
-->
