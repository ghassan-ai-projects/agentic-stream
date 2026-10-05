# Findings

Baseline `6750c10`. Paths are relative to the repository root. Findings are
grouped by the engineering principle they break, most important first.

## F1. Business rules, SQL and orchestration share function bodies

The rules of the context cannot be read or tested without a database.

| Rule | Where it lives today |
| --- | --- |
| A new boot requires reconciliation; the same boot keeps its status | `internal/authority/reconciliation.go` `updateReconciliationBoot` (Go) **and** `updateBoundDeviceStateSQL` (four `CASE WHEN boot_id = ?` clauses) |
| A claim is live while active and unexpired | `authority_claim.go` `live()` **and** `authority_lifecycle.go` `assertActiveClaimTx` (parses the lease again and re-implements `live`) |
| Only the exact holder may release an active claim | `authority_lifecycle.go` `markClaimReleased` (the rule is the `WHERE` clause) |
| A binding is idempotent only for the identical device boot, owner and digest | `authority_binding.go` `commandAlreadyBound` (scan and comparison in one function) |
| Resolution needs an open reconciliation, the latest state digest, and no unresolved command | `reconciliation_resolve.go` `assertResolvableBarrier` (query, comparison and second query in one function) |

Duplicated rules drift. The `CASE WHEN` clauses are a second, silent
implementation of "a reboot discards the previous resolution".

## F2. The public surface leaks internal structure

- Three entry types — `TargetAuthority`, `ReconciliationStore`, `SafetyLedger`
  — are built as struct literals with exported fields (`DB`, `Owner`,
  `EpochControl`, `InstanceID`, `Lease`, `Now`, `Authority`).
- Because any half-configured value is constructible, nearly every method
  starts with a nil guard (`if s == nil || s.DB == nil || s.Authority == nil`).
  Tests build `ReconciliationStore{DB: db}` with no authority at all.
- `internal/device/serial_session.go:96-129` reads `Authority.DB`,
  `Authority.Owner.DB`, `Authority.EpochControl.DB` and `Authority.InstanceID`
  to prove that the two authority objects agree. That check belongs to a
  constructor.
- Device tests run raw SQL through `authority.DB`
  (`internal/device/phase04_test.go:200`, `serial_effector_test.go:326`).
- `VerifyStoredJSONDigest` is a canonical-JSON utility exported from here only
  because `internal/soak` needed it.

## F3. Safety gates are optional

`TargetAuthority.Owner` and `TargetAuthority.EpochControl` may be nil; ordinary
admission then silently skips the runtime-owner and epoch checks
(`authority.go` `assertOrdinaryTx`). Production wiring always sets them, and
`device` re-checks that it did. A new composition or test harness that forgets
them turns the fence off without any error.

## F4. Other modules use the schema as the interface

| Reader | Table | Purpose |
| --- | --- | --- |
| `internal/actions/dispatch_reconcile.go:152` | `device_command_bindings` | business check: evidence for a device-bound command |
| `internal/soak/report_compute.go` | `device_safety_events`, `device_reconciliation`, `device_authority_events` | soak report |
| `internal/runartifact/export_*.go` | all five tables | run-artifact export |

In the opposite direction, `internal/authority/reconciliation_resolve.go`
`countUnresolvedCommands` reads `commands`, which `actions` owns, and hard-codes
the action ledger's status vocabulary.

The `actions` read is business logic and moves behind the authority API in this
work. The soak and export reads are read models over durable evidence; they are
recorded as follow-ups. The reverse read needs a port that `actions`
implements, which is a wiring change across `cmd`, `runtime` and `actions`; it
is also a follow-up.

## F5. Vocabulary drift

| Same concept | Names in use |
| --- | --- |
| The runtime process generation that holds the lease | `AuthorityEpoch`, `authorityEpoch`, `owner_epoch`, `authority_epoch`, `epoch` |
| One power-on of a device | "device lifetime", "boot", `bootID` |
| Commands are blocked until reconciled | "barrier open", "required", `Require` (verb), `Required` (predicate) |
| Recording the device's reported state | `BindState`, "bound state", `stateBinding` |
| Claim assertion | `Assert` (claim) vs `AssertRuntime` (runtime) on the same type |
| A safe stop was recorded for this boot | `SafeStopRequested` (also the name of a stage) |

"Authority" alone means three things: the package, the runtime owner, and a
target claim. `ReconciliationStore` is not a store; it holds the reconciliation
rules.

## F6. Redundant or misleading storage

- `device_reconciliation.opening_boot_id` always equals `boot_id`. Every write
  path sets it to the incoming boot.
- `device_reconciliation.opened_at` is written once, when the device's first
  state is recorded, and never when a reconciliation opens.
- `device_reconciliation.authority_epoch` holds the owner epoch that every other
  table calls `owner_epoch`.

## F7. Smaller defects

- `BindState` and `Resolve` check ordinary admission twice: once in a separate
  transaction through `AssertRuntime`, then again inside the real transaction.
- `releaseClaimTx` reads the clock twice, so the claim row and its audit event
  get different timestamps.
- `SafetyLedger.Record` calls `time.Now()` directly instead of the injected
  clock.
- `Resolve` reports `ErrReconciliationRequired` when **no** reconciliation is
  open, which is the opposite of what the error says.
- Device-wide audit events build a `TargetClaim` whose target is the device ID,
  with no name for that convention.
- `fmt.Errorf("%w", err)` wraps nothing (`authority.go`, `authority_binding.go`). It exists only to satisfy `wrapcheck`; the same pattern appears 28 times across `internal/`.
- `device` keeps a fallback `ReconciliationStore.RecordSafeStop` path that is
  unreachable because the session already requires `TargetAuthority`.

## F8. Observations outside this package

- No production code writes `device_safety_events`; only tests do. The soak
  verdict depends on an external evidence bridge that does not exist yet.
- Every importer aliases the package as `deviceauthority`, which suggests the
  package name is too generic. Renaming the directory is deferred because it
  touches the public architecture documentation.
