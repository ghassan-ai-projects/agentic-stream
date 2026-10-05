# Plan

Each round is reviewed, tested with focused tests, and committed. `make
ci-check` runs before handoff.

## Status

| Round | Commit | Notes |
| --- | --- | --- |
| R0 | `410cf4e` | |
| R1 | `5c23563` | |
| R2 | `d821f2d` | domain coverage 97.8% |
| R3 + R4 | `0b4df2b` | Merged: the store became the only table owner in the same commit that removed the old writers, so the ownership gate never saw two writers. Coverage: authority 86.8%, store 80.6%. |
| R5 | `36b246f` | |
| R6 | final round | `TestDomainPackagesArePure` and `TestModuleSQLStaysInStore` fail on an injected `time.Now`, an `os` import, and a SQL literal outside the store. |

Found during implementation and applied:

- The `%w`-only wraps existed to satisfy `wrapcheck`. A narrow linter
  exemption for a module's own layers replaces them (see the design).
- `ReleaseClaim` never had ordinary admission; it is now explicitly on the
  priority path, with a test that an owner can release after losing its lease.
- `TestRuntimeRecoveryRechecksLeaseBeforeCommitting` tested `control`, not
  `authority`; it moved to `internal/control`.
- `device` also renamed its `AuthorityEpoch` configuration to `OwnerEpoch`.
- A `device` test built a session with no authority and relied on nil-receiver
  checks; `device` now fails closed with an explicit error at that call.
- Soak tests create reconciliation state through the API instead of raw SQL,
  so they no longer depend on column names.

## Rounds

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

1. Device calls `AssertRuntime` before `RecordDeviceState` and
   `ResolveReconciliation`, which already admit inside their transaction. The
   extra check only shapes device error messages and session state; remove it
   when `device` is refactored.
2. Replace the 28 remaining `fmt.Errorf("%w", err)` no-op wraps elsewhere in
   `internal/`.
3. Replace the authority read of `commands` with a port that `actions`
   implements (needs `cmd`/`runtime` wiring).
4. Give `soak` and `runartifact` read APIs instead of direct table reads.
5. Rename the package to `deviceauthority`.
6. Replace `map[string]any` device state and evidence with typed records.
7. Apply the [module pattern](MODULE_PATTERN.md) to `control`, `approvalledger`
   and `qualification`.
