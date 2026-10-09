# authority

Status: done
Round: 11

Audited in round 11 with [policy](policy.md). `authority` is the repository's
reference module (see its
[module pattern](../../authority-reference-module-2026-10-05/MODULE_PATTERN.md)),
so the bar here was "a test other modules can copy". Production code is
unchanged.

## Metrics

Times are `go test -short -race -count=1` wall time per package, measured while
other workers were running. Tests are top-level tests / passing subtests.

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/authority` (facade) | 100.0% | 100.0% | 1.6 s | 1.5 s | 3 / 8 | 3 / 8 |
| `internal/authority/internal/app` | 84.7% | 87.0% | 1.8 s | 2.0 s | 18 / 0 | 22 / 2 |
| `internal/authority/internal/domain` | 97.4% | 97.4% | 1.3 s | 1.1 s | 19 / 55 | 28 / 55 |
| `internal/authority/internal/store` | 81.2% | 84.1% | 1.6 s | 1.6 s | 9 / 0 | 15 / 3 |

The module was already parallel (0 hygiene findings before and after) and fast
(no test above 0.3 s). Repository `golangci-lint`: 0 issues. `dupl` at threshold
60: 0. `time.Sleep`, `context.Background()`, `t.Skip`: none.
`go test -race -shuffle=on -count=10` on app, store and the facade: passes.
Mutation check (reverted): `After` to `!Before` in the claim lease test fails
tests in the facade, app and domain packages.

## Findings and changes

The module's layering was already right (domain tables, store round trips, app
use cases over a real database because the transaction is the behavior, a
facade that drives every operation once). What fell short of "exemplary":

### Removed
- `db.SetMaxOpenConns(1)` in the three test fixtures and the unused `name`
  parameter of the facade's `openDB`: the single-connection pin hid nothing (the
  tests pass 10 times with shuffling on the default pool), and `openDB` was just
  `storagetest.OpenTemp`.
- `internal/store/store_test.go`: split by subject (below).

### Renamed or moved
- store `store_test.go` → `fixture_test.go`, `unit_of_work_test.go`,
  `claims_test.go`, `bindings_test.go`, `events_test.go`, `safety_test.go`
  (`reconciliation_test.go` stays).
- 15 domain test names and 6 store test names were verbs over a function
  (`TestDecideClaim`, `TestObserveState`, `TestCheckResolvable`,
  `TestClaimsRoundTrip`, `TestReconciliationLifecycle`, ...); they are sentences
  about behavior now, for example
  `TestAClaimDecisionFollowsTheHeldClaimAndItsLease`,
  `TestOnlyTheCurrentBootOfAKnownDeviceCanHaveAReconciliationOpened`,
  `TestReconciliationMovesFromFirstStateThroughManualReviewToAReboot` (T3).

### Improved
- T11/T4: tests that proved several unrelated things in one body with
  `t.Fatal("rule name")` were split: command-binding completeness, the device
  subject, the safe-stop event, explicit occurrence time, physical-evidence
  completeness, the reboot event, the opening event, "an opening needs a reason"
  and "resolution waits for every bound command" each have their own test with
  got/want messages.

### Added
- `TestADrainingOrKilledEpochRefusesOrdinaryOperationsButNotThePriorityPath`
  (app): after the control plane drains or kills the epoch, `AssertRuntime`,
  `Claim`, `AssertClaim` and `BindCommand` return `ErrEpochDraining` or
  `ErrEpochKilled`, while `RecordSafeStop` and `ReleaseClaim` still succeed. This
  was an unexercised branch of ordinary admission (`tx.go`), and the safety
  property of the priority path.
- app: `TestTheServiceReportsTheOwnerInstanceItWasConfiguredFor` (was 0%),
  `TestADeviceStateWithoutADeviceAndBootIsRefusedAndRecordsNothing`,
  `TestAskingWhetherADeviceRequiresReconciliationNeedsADeviceId`.
- store: `TestAnUnclaimedTargetHasNoClaim`,
  `TestTheNextAcceptedDecisionReplacesTheClaimOnATarget`,
  `TestAClaimWithAnUnreadableLeaseIsRefused`, `TestACommandIsBoundToADeviceOnce`,
  `TestTheAuthorityAuditLogIsCounted` (`CountAuthorityEvents` was 0%),
  `TestSafetyEvidenceThatNoLongerVerifiesIsRefusedWhicheverPartChanged`
  (rewritten details, cleared digest, corrupt time; previously only the first).

### Speed
- Nothing was slow.

## Production code touched
- none.

## Invariants proven here
- 8 (stable identities, idempotency, no duplicate external work after a crash):
  `TestBindCommandIsIdempotentAndRefusesConflicts`,
  `TestRebootRequiresReconciliationAcrossRestartAndManualReview` (the barrier
  survives a restart), `TestResolutionWaitsOnlyForTheSameDeviceBootsCommands`,
  `TestACommandIsBoundToADeviceOnce` (store), `TestAClaimDecisionFollowsTheHeldClaimAndItsLease`
  (fence advances on takeover), `TestClaimFencesAnotherOwnerUntilTheClaimExpires`,
  `TestAssertClaimRequiresTheExactLiveHolder`.
- Safety evidence is durable and tamper-evident (supports 10):
  `TestSafetyEvidenceThatNoLongerVerifiesIsRefusedWhicheverPartChanged`,
  `TestReadSafetyRecordSummarizesDurableEvidence`.
- Authority loss and drain: `TestAssertRuntimeFollowsTheRuntimeLease`,
  `TestReleaseStaysAvailableAfterAuthorityLoss`,
  `TestSafeStopLatchesTheBootOnThePriorityPath`,
  `TestOpenReconciliationAfterAuthorityLossUsesThePriorityPath`,
  `TestADrainingOrKilledEpochRefusesOrdinaryOperationsButNotThePriorityPath`.

## Open items
- `internal/authority/internal/store/reader.go` defines its own `nullable[T]`
  while AGENTS.md names `storage.NullIfEmpty` for empty-string-to-NULL. Production
  code, outside a test round; worth one small change.
- The app layer's remaining uncovered lines (87.0%) are the wrapped returns of a
  failing store call in the middle of an operation (for example a claim write that
  fails after the load). Provoking each needs a trigger or a dropped table per
  step; the unit-of-work rollback is proven once in the store
  (`TestUnitOfWorkRollsBackTogether`) and per operation where an audit failure
  matters (`TestReleaseRollsBackWhenItsAuditCannotBeWritten`,
  `TestOpeningRollsBackWithoutItsAudit`).
