# Plan

Each round is reviewed, tested with focused tests, and committed. `make
ci-check` runs before handoff.

| Round | Change | Proof |
| --- | --- | --- |
| R0 | This folder: findings, language, design, pattern, plan. | Review |
| R1 | `canonicaljson`: add `EncodeDigest` (inverse of `DecodeDigest`) and `VerifyStored`; `soak` uses `VerifyStored`; remove `authority.VerifyStoredJSONDigest`. | `canonicaljson`, `soak` tests |
| R2 | `internal/authority/internal/domain`: vocabulary and pure rules, with table-driven tests. | `go test ./internal/authority/...` |
| R3 | `internal/authority/internal/store`: all SQL, with tests against a migrated SQLite database. | store tests |
| R4 | Rebuild `internal/authority` on `domain` + `store`: `Service`, `New(Config)`, renamed operations, typed outcomes and stages, single admission per transaction, single clock read, `ErrNoOpenReconciliation`. Move the `actions` binding check behind `VerifyCommandEvidence`. Update `device`, `actions`, `soak`, `cmd` and all tests. | authority, device, actions, soak, cmd tests |
| R5 | Migration `031`: drop `opening_boot_id`, rename `authority_epoch` → `owner_epoch`, `opened_at` → `first_seen_at`. | migration and authority tests |
| R6 | Architecture gates: ownership pinned to the store, `TestDomainPackagesArePure`, `TestModuleSQLStaysInStore`, layers and import allowlist. Update the repository map, module docs and agent context to name this module as the reference. | `go test .`, `make docs-check` |

## Behavior that stays the same

- Claim decisions, fences, lease handling, idempotent and conflicting bindings,
  reboot detection, reconciliation opening and resolution rules, safe-stop
  latching, safety-event validation.
- Every state change and its audit event commit or roll back together.
- Ordinary admission runs before any read or write in the operation's
  transaction. The priority path still skips it.
- Read-only queries (`ReconciliationRequired`, `SafeStopLatched`) still run
  without a write transaction.

## Deliberate behavior changes

- `New` refuses a missing runtime owner or epoch control (previously skipped).
- Owner instance comes from the runtime owner; every operation checks it.
- `ResolveReconciliation` on a clear device returns `ErrNoOpenReconciliation`.
- Admission runs once per operation, inside the operation's transaction.
- The reboot audit event carries a `reason`; `opening_boot_id` is gone.
- Safety events default their time from the configured clock.

## Deferred follow-ups

1. Replace the authority read of `commands` with a port that `actions`
   implements (needs `cmd`/`runtime` wiring).
2. Give `soak` and `runartifact` read APIs instead of direct table reads.
3. Rename the package to `deviceauthority`.
4. Replace `map[string]any` device state and evidence with typed records.
5. Apply the [module pattern](MODULE_PATTERN.md) to `control`, `approvalledger`
   and `qualification`.
