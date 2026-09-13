# Full Repository Audit Gauntlet Bar

Status: active
Owner: lead agent
Scope: every actionable finding recorded by `INDEX.md` and its linked audit files.

## Fixed goal

Resolve every audit finding without weakening the ten product invariants, while
leaving the repository in a production-quality, deterministic, test-proven
state.

## External bar

The goal is met only when all of the following are evidenced:

1. Every actionable finding in `INDEX.md` has a passing acceptance bar, a
   precise resolution receipt, and no unresolved finding remains.
2. All release-blocking invariants remain true, including fail-closed input
   handling, immutable Situation versions, serial deterministic state,
   bounded episodes, policy-before-effect, durable identity/idempotency, and
   effect-free replay.
3. Production code has no swallowed correctness errors, dead exported
   production surface, silent schema/runtime no-ops, divergent duplicate
   implementations, unsafe context/time behavior, or unbounded resource path
   identified by the audit.
4. Tests assert behavior rather than execution, cover every repaired failure
   path, are deterministic and bounded, and do not depend on adjacent private
   repositories or network access.
5. The storage contract, migrations, embedded schemas, compiler, runtime, and
   public documentation describe one achievable implementation state.
6. `make ci-check`, `go test ./...`, `go vet ./...`, `git diff --check`, and the
   repository's documentation checks pass; optional-tool skips are explicitly
   recorded rather than treated as success.
7. A fresh lead context can reproduce the latest judgment from the committed
   artifacts and continue from the Ready Frontier without resetting gain or
   plateau history.

## Boundaries

- Highest Go quality; prefer the smallest design-consistent change.
- No new architecture, dependency, provider, network call, or feature scope.
- No secrets, destructive resets, or unrelated refactors.
- Preserve existing user changes; do not overwrite unrelated work.
- Production changes require proving tests in the same round.
- Commit after every accepted round.
- A review result is not approval unless its evidence is reproducible in the
  current artifact and working tree.

## Normalized scenes

- `runtime-safety`: event ingestion, deterministic operators, situations,
  cognition, bounded episodes, policy, actions, and replay preserve invariants.
- `durability`: cancellation, leases, fencing, transactions, recovery,
  idempotency, and error propagation preserve progress and truthful outcomes.
- `contract`: schemas, migrations, wire contracts, compiler behavior, and
  runtime support are mutually consistent and fail closed.
- `quality`: dead code, duplicated logic, time/context discipline, tests,
  assertions, and public surfaces are simple and maintained.
- `artifact`: audit files, checkpoint receipts, commits, and final evidence
  are complete enough for independent resumption.

## Judgment rule

An item is accepted only after the changed artifact is inspected, the narrow
proving tests pass, the relevant quality lenses have challenged it, and the
resolution is recorded in the linked audit file. A full-artifact judgment is
required before declaring the goal complete.
