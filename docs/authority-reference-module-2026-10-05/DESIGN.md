# Target design

## Layers

```
            callers (device, actions, soak, cmd)
                         │  public API only
                         ▼
┌────────────────────────────────────────────────────────────┐
│ internal/authority            application layer + API       │
│  Service, Config, New; value types (aliases); sentinel     │
│  errors. Each operation: validate → open transaction →     │
│  admit → load → decide (domain) → persist (store) → audit. │
└───────────────┬───────────────────────────┬────────────────┘
                │                           │
                ▼                           ▼
┌──────────────────────────────┐ ┌──────────────────────────────┐
│ internal/domain              │◄┤ internal/store               │
│ vocabulary and rules; pure   │ │ SQL only; maps rows to and   │
│ functions over values        │ │ from domain values           │
└──────────────────────────────┘ └──────────────────────────────┘
```

Dependencies point inward: the application layer uses `domain` and `store`;
`store` uses `domain`; `domain` uses neither. `domain` and `store` sit under the
module's own `internal/` directory, so no other package can import them.

### Domain (`internal/authority/internal/domain`)

- Holds the vocabulary from the [ubiquitous language](UBIQUITOUS_LANGUAGE.md)
  as types, constants and sentinel errors.
- Holds every decision as a pure function: claim decisions and fences, claim
  checks, binding comparison, the device-state transition, reconciliation
  opening and resolution checks, evidence validation, safe-stop and safety-event
  validation.
- Takes time as a parameter. It never reads a clock, opens a transaction,
  imports `database/sql`, or performs I/O.
- Computes digests that are part of a rule (device-state and evidence digests),
  because evidence binding compares them.

### Store (`internal/authority/internal/store`)

- Owns every SQL statement for `device_target_claims`, `device_command_bindings`,
  `device_reconciliation`, `device_authority_events` and `device_safety_events`.
- Mutations take a `*sql.Tx`. A write cannot run outside a transaction.
- Reads take a small `Reader` interface (`QueryRowContext`), so a read runs in
  the caller's transaction when it must, or on the database handle when it is
  a standalone query.
- Owns storage encodings: time format, digest bytes, canonical event details.
- Contains no decisions. A `WHERE` clause selects rows; it does not decide
  whether an operation is allowed.

### Application layer and public API (`internal/authority`)

| File | Holds |
| --- | --- |
| `api.go` | value-type aliases, outcome and stage constants, sentinel errors, stateless rules |
| `service.go` | `Config`, `New`, `Service`, identity checks, `AssertRuntime` |
| `tx.go` | `withAdmittedTx`, `withPriorityTx`, `admitOrdinary`, the clock read |
| `claims.go`, `commands.go`, `reconciliation.go`, `resolution.go`, `safety.go` | one file per operation group |

One entry type:

```go
func New(cfg Config) (*Service, error)

type Config struct {
	DB         *storage.DB
	Owner      *control.RuntimeOwner // required: runtime-owner fence
	Epochs     *control.EpochControl // required: drain/kill fence
	ClaimLease time.Duration         // default one minute
	Clock      clock.Clock           // default physical clock
}
```

`New` refuses a missing database, owner or epoch control, and refuses
components that use different databases. After `New` succeeds, no method needs
a nil guard.

| Group | Operation |
| --- | --- |
| Identity | `OwnerInstance() string` |
| Admission | `AssertRuntime(ctx, epoch)` |
| Claims | `Claim`, `AssertClaim`, `ReleaseClaim` (each takes a `TargetClaim`) |
| Commands | `BindCommand(ctx, CommandBinding)`; package function `VerifyCommandEvidence(ctx, tx, commandID, target, evidence)` |
| Reconciliation | `RecordDeviceState(ctx, owner, state)`, `ReconciliationRequired(ctx, deviceID)`, `OpenReconciliation(ctx, device, owner, reason)`, `OpenReconciliationAfterAuthorityLoss(...)`, `ResolveReconciliation(ctx, device, owner, outcome, evidence)`; package function `ValidateReconciliationEvidence(evidence, device)` |
| Safety | `RecordSafeStop(ctx, claim, stage, details)`, `SafeStopLatched(ctx, device)`, `RecordSafetyEvent(ctx, event)`; package function `PhysicalEvidenceComplete(details)` |

Value types (`Owner`, `DeviceBoot`, `TargetClaim`, `CommandBinding`,
`SafetyEvent`, `ResolutionOutcome`, `SafeStopStage`) are aliases of domain types.
The aliases expose data, not the domain's internal model: `HeldClaim`,
`Reconciliation`, `AuthorityEvent` and the decision types are not re-exported.

The package functions are stateless rules or reads that run inside another
module's transaction. `VerifyCommandEvidence` takes the caller's `*sql.Tx`
because `actions` verifies evidence inside its own reconciliation transaction.

## Rules every operation follows

1. **Validate first.** Identity completeness and owner-instance checks run
   before any transaction.
2. **One transaction per operation.** Admission, load, decision, persistence and
   audit share it, so an audit failure rolls the state change back.
3. **Admission is the first statement in the transaction.** Ordinary operations
   use `withAdmittedTx`; priority-path operations use `withPriorityTx`, which
   is the only place admission is skipped. The priority path is
   `RecordSafeStop`, `OpenReconciliationAfterAuthorityLoss`, `ReleaseClaim` and
   `RecordSafetyEvent`.
4. **One clock read per operation**, taken before the transaction, from the
   configured `clock.Clock`.
5. **Every state change is audited** in the same transaction.
6. **Errors:** sentinel errors for outcomes callers branch on. Each public
   operation wraps once with its name (`claim target "fan-01": …`). Errors
   from the module's own `domain` and `store` pass through unwrapped; `store`
   names the failed statement itself. `wrapcheck` is configured to accept this
   (`ignore-package-globs: */internal/*/internal/*`) and to accept
   `storage.DB.WithTx`, which returns its callback's error unchanged.

## Enforcement

| Rule | Check |
| --- | --- |
| Only `store` writes the module's tables | `durableOwners` in `architecture_ownership_test.go` names `internal/authority/internal/store` |
| A package named `.../internal/domain` is pure: no `database/sql`, `net`, `os`, storage or control imports, and no `time.Now` | new `TestDomainPackagesArePure` |
| SQL text in a module that has a `.../internal/store` package lives only in that store | new `TestModuleSQLStaysInStore` |
| Layer order `domain` < `store` < `authority` | `packageLayers`, `allowedImports` |
| No other module imports `domain` or `store` | Go's `internal/` rule (compiler) |
| `New` refuses missing safety dependencies | unit tests in `internal/authority` |

## Schema changes

Migration `031_device_reconciliation_language.sql` aligns storage with the
language:

- drop `device_reconciliation.opening_boot_id` (always equal to `boot_id`);
- rename `device_reconciliation.authority_epoch` to `owner_epoch`;
- rename `device_reconciliation.opened_at` to `first_seen_at`, which is what it
  records.

The reboot audit event carries `{"reason": "device_rebooted",
"previous_boot_id": …}`, so every `reconciliation_opened` event has a reason.
