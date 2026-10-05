# Target design

## Layers

Four layers, each with one responsibility:

```
          callers (device, actions, soak, cmd)
                       │  public API only
                       ▼
 internal/authority                  FACADE: API, configuration, delegation
   Service, Config, New, aliases, errors
                       │
                       ▼
 internal/authority/internal/app     LOGIC: use cases
   validate → unit of work → admit → load → decide → persist → audit
              │                               │
              ▼                               ▼
 internal/authority/internal/domain   internal/authority/internal/store
 RULES: pure functions over values    DATABASE: transactions and SQL
              ▲                               │
              └───────────────────────────────┘
                   store maps rows to domain values
```

Dependencies point one way: facade → app → domain and store; store → domain.
`app`, `domain` and `store` sit under the module's own `internal/` directory,
so no other package can import them.

| Layer | Responsibility | Must not |
| --- | --- | --- |
| Facade (`internal/authority`) | Public types, `New(Config)` and its validation, one-line delegation of each operation | hold logic, SQL or transactions |
| Logic (`internal/app`) | Use cases: input checks, which operations need admission, running admission, ordering load → decide → persist → audit | import `database/sql` or `storage`; contain SQL |
| Rules (`internal/domain`) | Vocabulary and every decision as a pure function | perform I/O, read a clock, open a transaction |
| Database (`internal/store`) | Transactions (`InTx`), units of work (`Tx`), every SQL statement, storage encodings | decide anything |

### Facade (`internal/authority`)

| File | Holds |
| --- | --- |
| `api.go` | value-type aliases, outcome and stage constants, sentinel errors, stateless rules |
| `service.go` | `Config`, its validation and defaults, `New`, `Service` |
| `operations.go` | one documented line per public operation, delegating to `app` |

```go
func New(cfg Config) (*Service, error)

type Config struct {
	DB         *storage.DB
	Owner      *control.RuntimeOwner // required: runtime-owner fence
	Epochs     *control.EpochControl // required: drain/kill fence
	Outcomes   OutcomeLedger         // required: actions.CountUnresolvedOutcomes
	ClaimLease time.Duration         // default one minute
	Clock      clock.Clock           // default physical clock
}
```

`Config.DB` is the facade's only contact with the database: it is handed to
`store.New`. `New` refuses a missing database, owner or epoch control, and
components on different databases, so after `New` no operation needs a nil
guard.

| Group | Operation |
| --- | --- |
| Identity | `OwnerInstance() string` |
| Admission | `AssertRuntime(ctx, epoch)` |
| Claims | `Claim`, `AssertClaim`, `ReleaseClaim` (each takes a `TargetClaim`) |
| Commands | `BindCommand(ctx, CommandBinding)`; package function `VerifyCommandEvidence(ctx, tx, CommandEvidence)` |
| Reconciliation | `RecordDeviceState(ctx, owner, state)`, `ReconciliationRequired(ctx, deviceID)`, `OpenReconciliation(ctx, ReconciliationOpening)`, `OpenReconciliationAfterAuthorityLoss(ctx, ReconciliationOpening)`, `ResolveReconciliation(ctx, ResolutionRequest)` |
| Evidence | `SealReconciliationEvidence(source, target, state, feedback)`, `ParseReconciliationEvidence(document, device)`; `ReconciliationEvidence.Document()` is the wire form |
| Safety | `RecordSafeStop(ctx, claim, stage, details)`, `SafeStopLatched(ctx, device)`, `RecordSafetyEvent(ctx, event)`; package functions `ReadSafetyRecord(ctx, tx)` and `PhysicalEvidenceComplete(details)` |

Operations with more than a few inputs take one request value
(`ReconciliationOpening`, `ResolutionRequest`, `CommandEvidence`), so call
sites name every field.

Evidence documents are parsed once at the boundary into a typed
`ReconciliationEvidence` with a closed set of fields; the rules read typed
fields. The authority also seals evidence, so the producer (`device`) and the
verifier share one digest scheme.

Value types are aliases of domain types; the domain's internal model
(`HeldClaim`, `Reconciliation`, `AuthorityEvent`, decisions) is not exported.
`VerifyCommandEvidence` takes the caller's `*sql.Tx` because `actions` verifies
evidence inside its own transaction; the facade only wraps it with
`store.Join` and delegates.

### Logic (`internal/authority/internal/app`)

| File | Holds |
| --- | --- |
| `service.go` | `Config`, `Fences`, `Service`, identity checks, `AssertRuntime`, the clock read |
| `tx.go` | `inAdmittedTx`, `inPriorityTx`, `admitOrdinary` |
| `claims.go`, `commands.go`, `reconciliation.go`, `resolution.go`, `safety.go` | one file per use-case group |

Admission is logic: the rule "ordinary operations run behind the
runtime-owner and epoch fences; the priority path does not" lives here. The
fences themselves belong to `control`; `app` receives them as `store.Fence`
values and runs them as the first step of the unit of work.

### Rules (`internal/authority/internal/domain`)

- The vocabulary from the [ubiquitous language](UBIQUITOUS_LANGUAGE.md) as
  types, constants and sentinel errors.
- Every decision as a pure function: claim decisions and fences, holder checks,
  binding comparison, the device-state transition, reconciliation opening and
  resolution checks, evidence validation, safe-stop and safety-event
  validation.
- Time arrives as a parameter. Digests that a rule compares (device state,
  evidence) are computed here.

### Database (`internal/authority/internal/store`)

- `Store.InTx` opens a transaction and hands the use case a `Tx`; `Join` wraps
  a transaction another module opened.
- `Tx` methods are named after domain actions (`LoadClaim`, `RecordReboot`,
  `AppendAuthorityEvent`) and hold every SQL statement for the module's five
  tables. `Store` serves the two standalone reads that must not take the write
  lock.
- `Tx.Assert(ctx, fence, epoch)` and `Tx.CountUnresolved(ctx, ledger, ids)`
  run another module's transactional check or read on the open transaction.
  They forward; they do not decide. The store reads only its own tables: it
  lists the commands bound to a device boot, and `actions` (through the
  ledger) says which of them are unresolved.
- Owns storage encodings: time format, digest bytes, canonical event details.

## Rules every operation follows

1. **Validate first.** Identity completeness and owner-instance checks run
   before any transaction.
2. **One unit of work per operation.** Admission, load, decision, persistence
   and audit share one store transaction, so an audit failure rolls the state
   change back.
3. **Admission is the first statement in the transaction.** Ordinary operations
   use `inAdmittedTx`; priority-path operations use `inPriorityTx`, which
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
| The logic layer does not touch the database | new `TestApplicationLayersDoNotTouchTheDatabase` (no `database/sql` or `storage` in `.../internal/app`) |
| A package named `.../internal/domain` is pure: no `database/sql`, `net`, `os`, storage or control imports, and no `time.Now` | new `TestDomainPackagesArePure` |
| SQL text in a module that has a `.../internal/store` package lives only in that store | new `TestModuleSQLStaysInStore` |
| Layer order `domain` < `store` < `app` < `authority` | `packageLayers`, `allowedImports` |
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
