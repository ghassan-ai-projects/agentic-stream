# actions

Status: done
Round: 12

Audited in round 12 with [actionport](actionport.md), [watch](watch.md) and
[notify](notify.md). Production code is unchanged. The ledger rules that make
an effect happen at most once now each have a named test at the layer that owns
them, including the crash between the effect and its recorded outcome, which
had no test before.

## Metrics

Times are `go test -short -race -count=1` wall time per package, measured while
other workers were running. Tests are top-level tests / passing subtests.

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/actions` (facade) | 100.0% | 100.0% | 1.9 s | 1.7 s | 5 / 0 | 5 / 0 |
| `internal/actions/internal/app` | 81.9% | 89.4% | 3.6 s | 3.7 s | 21 / 14 | 26 / 48 |
| `internal/actions/internal/domain` | 97.7% | 99.4% | 1.2 s | 1.3 s | 23 / 49 | 26 / 77 |
| `internal/actions/internal/store` | 76.7% | 82.2% | 2.7 s | 3.1 s | 22 / 10 | 30 / 15 |

No test is above 0.75 s and no package above 3.8 s. App and store hold more
tests, each on its own migrated temporary database, so their wall time is flat.
`dispatch_test.go` (735 lines) and `store_test.go` (543) are gone; the largest
test file is now 215 lines (the shared fixture).

Hygiene lint (paralleltest, tparallel, usetesting, thelper): 19 findings before
in `internal/actions/internal/app`, 0 after. Repository `golangci-lint`: 0
issues. `dupl` at threshold 60 in test files: 0 (the production twins
`scanCommandView`/`scanAwaitingCommand` stay, as AGENTS.md says).
`time.Sleep` and `t.Skip` in tests: none. `time.Now()`: one in `service_test.go`
(the default-clock test needs wall time) and one in
`device_verification_test.go` (compares against a real context deadline);
everything else runs on a virtual clock or the fixed `fixtureNow`.
`go test -race -shuffle=on -count=3`: passes.

Mutation checks (one production line changed, tests run, line restored): treating
an expired lease as acquirable, dropping the `!held` drop of a late result, removing
the lease-owner guard of the outbox close, offering delivered outbox rows again,
skipping the approval check and skipping the policy-digest check each fail a named
test.

## Findings and changes

### Removed
- `TestExpiredLeaseIsObserved` (app, `service_test.go`): asserted only
  `observer.expiries == 0` was false (T4). Folded into the lease table, which now
  asserts the observed expiry next to the ledger.
- `TestDispatcherBoundsVerificationByDispatchLease` (app): slept on a real 50 ms
  lease and asserted `elapsed < 1s` (T5). Replaced by a deterministic check of the
  context deadline given to the dispatch call and the device query.
- `TestInterlockTripRefusesTheCommand`, `TestUnjoinedTransactionRefusesTheOwnerCheck`,
  `TestAssertOwnerNamesTheLostOwnership` (store): three tests of two methods,
  merged into `TestRuntimeOwnershipIsAssertedOnTheTransactionOrRefused` and
  `TestATrippedInterlockRefusesTheCommandAndTheDispatchAuthorization`.
- `TestDeviceVerificationOutcomes` (domain): four unrelated asserts in one body,
  now a four-row table.
- `defer db.Close()` noise in every app test: `storagetest.OpenTemp` closes with
  the test.

### Renamed or moved
App (every old test and where it lives now):

| Old | New |
| --- | --- |
| `dispatch_test.go` | split into `fixture_test.go`, `effectors_test.go`, `dispatch_outcome_test.go`, `dispatch_notification_test.go`, `device_verification_test.go`, `authorization_test.go`, `crash_recovery_test.go` |
| `TestDispatcherRecordsSuccessAndDoesNotRedispatchDeliveredOutbox`, `TestDispatcherDoesNotBlindlyRetryUnknownOutcome` (ledger half), `TestDispatcherKeepsAcceptedTransportAwaitingVerification` (ledger half), `TestDispatcherTreatsProviderDeadlineAsUnknownWithoutRetry` | `TestADispatchRecordsItsOutcomeAndTheEffectIsNeverRepeated` (5 outcome kinds) |
| notification halves of the same tests | `TestASuccessfulDispatchPublishesItsRecordedAndReconciledOutcome`, `TestAnUnknownOutcomeIsPublishedAsRecordedAndNeverAsReconciled` |
| reconcile halves | `TestAnUnknownOutcomeIsReconciledOnlyByTypedIndependentEvidence` (`reconciliation_test.go`) |
| `TestDispatcherVerifiesDeviceOutcomeAfterDispatch`, `...ReconcilesUnknownDeviceOutcomeFromState`, `...KeepsVerificationQueryFailureUnknown` | `TestDeviceStateDecidesTheOutcomeAfterADispatch` |
| `TestDispatcherRefusesCommandWhenInterlockTrips`, `...EffectorAcceptanceRechecksInterlock` | `TestATrippedInterlockFailsTheCommandBeforeTheEffector`, `TestTheInterlockIsCheckedAgainAtTheMomentTheEffectorAccepts` |
| `TestDispatcherReclaimsExpiredLease`, `TestDispatcherAbandonsUnreadableLeaseAsUnknownOutcomeWithoutError` | `TestALeaseThatCannotProveItIsLiveBecomesAnUnknownOutcomeWithoutAnEffectorCall` |
| `TestReconcilingAManualReviewCommandSettlesItsStatus` | `TestReconciliationSettlesTheCommandAndItsVerificationFromTheFinalStatus` (2 starts x 3 final statuses) |
| `service_test.go` `TestDispatchStopsWithoutRuntimeOwnership` | `TestADispatcherWithoutRuntimeOwnershipNeitherLeasesNorDispatches` (`authorization_test.go`) |
| `reconciler_test.go` | `reconciliation_test.go` (`TestTheReconcilerListsAnUnknownOutcomeAndClosesItOnce`, `TestAReconcilerRequiresAConfiguredStore`) |
| `fault_test.go` | `write_boundary_test.go` |

Store: `store_test.go` is `transaction_test.go`, `candidates_test.go`,
`authorization_test.go`, `outcome_test.go`, `unresolved_test.go` plus the shared
`fixture_test.go`; `lease_boundary_test.go` is merged into `lease_test.go`.

### Improved
- T6: every test and subtest in the module is parallel (19 findings to 0).
- T5: the fixture and every call take a fixed `fixtureNow` and a `sources.NewVirtual`
  clock; lease expiry is `clock.Advance`, not wall time. Store tests use a fixed
  `testNow` instead of `time.Now()`.
- T4: ledger assertions read one `ledger{Command, Outbox, Outcome, Reconciliation,
  Verification, Outcomes}` value and print it with the wanted value; error tests assert
  the refusal text (`reconciliation evidence source is required`, `is not awaiting
  reconciliation`, `command_digest_mismatch`, `interlock rejected command`).
- T9: one `scriptedEffector` (succeeds, fails with an error, runs a hook during the call)
  and one `deviceEffector` replace three hand-written effectors; one `newDispatcher` with
  options replaces `newService`/`newServiceWithOwner`; one `readLedger` replaces five
  copies of the same join query.
- T11: the 75-line `openActionFixture` is eight small steps; the 100-line success test is
  a table plus a notification test.

### Added
App (81.9% to 89.4%):
- `TestACrashBetweenTheEffectAndItsOutcomeIsNeverDispatchedAgain`: the effect runs, the
  outcome write dies, the lease expires, a restarted dispatcher records an unknown outcome
  and the effector is called once in total, also after a further dispatch.
- `TestAProviderResultThatArrivesAfterTheLeaseExpiredIsRecordedAsUnknown`: a success
  reported after the lease expired is stored as unknown, not as success.
- `TestAProviderResultFromADispatcherThatLostItsLeaseIsDropped`,
  `TestALeaseThatExpiresBeforeRevalidationIsRecordedAsUnknownWithoutAnEffect`.
- `TestAnOutboxRowReplayedForAFinishedCommandIsClosedWithoutCallingTheEffector`
  (succeeded and outcome-unknown commands).
- `TestAnIntentIsRevalidatedAgainstCurrentStateJustBeforeTheEffector`: 12 rows (policy no
  longer approves, rejected decision, newer material Situation version, running episode,
  expired intent, approval absent/unexpired/expired, policy digest matching/stale/missing);
  every refusal leaves the effector uncalled and a failed command.
- `TestATamperedCommandDocumentFailsTheCommandWithoutCallingTheEffector`,
  `TestOwnershipLostAfterLeasingStopsTheDispatchBeforeTheEffector`,
  `TestTheDispatchCallAndTheDeviceQueryBothRunUnderTheDispatchLeaseDeadline`,
  `TestAClosedCommandCannotBeReconciledTwice`.

Store (76.7% to 82.2%): `TestALeaseIsExclusiveUntilItExpires` (live, one nanosecond before
and at expiry), `TestTheNextCandidateIsTheOldestOutboxRowAvailableAtThatTime`,
`TestOnlyACommandNotYetSettledIsMarkedDispatching`,
`TestADispatcherThatLostItsLeaseCannotCloseTheOutboxOrOverwriteASettledCommand`,
`TestACommandHasOneIdempotencyKeyAndNoSecondLedgerEntryForAReplay`,
`TestAPolicyDigestIsReadOnlyFromAnApprovingEvaluationOfTheIntent`,
`TestReconciliationOnlyClosesACommandThatStillAwaitsIt`,
`TestTheReconciliationQueueListsAnUnresolvedCommandOfTheTenantOldestFirst`,
`TestIntentCommandsReadTheirOutcomesInOrderWithTheirProviderResults`.

Domain (97.7% to 99.4%): `TestAResultAfterTheLeaseExpiredIsAnUnknownOutcomeWithoutAnEffect`,
`TestAReconciliationOutcomeDocumentCitesItsFinalStatusAndEvidence`, four device-feedback
field rows in the evidence order table.

### Speed
- Nothing was slow. The only real-time wait left is `TestDeviceVerification...Deadline`, which
  does not wait at all.

## Production code touched
- none.

## Invariants proven here
- 8 and the MVP criterion "prevent duplicate ticket effects across crash and replay"
  (stable identities, outbox, idempotency):
  - crash between dispatch and outcome:
    `TestACrashBetweenTheEffectAndItsOutcomeIsNeverDispatchedAgain`,
    `TestALeaseThatCannotProveItIsLiveBecomesAnUnknownOutcomeWithoutAnEffectorCall`,
    `TestAProviderResultThatArrivesAfterTheLeaseExpiredIsRecordedAsUnknown`,
    `TestAProviderResultFromADispatcherThatLostItsLeaseIsDropped`;
  - dispatched at most once in every outcome (success, unknown, deadline, failure,
    pending verification): `TestADispatchRecordsItsOutcomeAndTheEffectIsNeverRepeated`;
  - a replayed command is recognized: `TestAnOutboxRowReplayedForAFinishedCommandIsClosedWithoutCallingTheEffector`,
    `TestACommandHasOneIdempotencyKeyAndNoSecondLedgerEntryForAReplay` (store),
    domain `TestAdmitDecidesEachCandidateKind`;
  - one dispatcher holds the row: `TestALeaseIsExclusiveUntilItExpires`,
    `TestADispatcherThatLostItsLeaseCannotCloseTheOutboxOrOverwriteASettledCommand`,
    `TestOnlyACommandNotYetSettledIsMarkedDispatching` (store);
  - an unknown outcome is never retried, only reconciled with independent evidence:
    `TestAnUnknownOutcomeIsReconciledOnlyByTypedIndependentEvidence`,
    `TestAClosedCommandCannotBeReconciledTwice`, domain `TestReconciliationEvidenceValidationOrder`.
- 7 (revalidation immediately before dispatch):
  `TestAnIntentIsRevalidatedAgainstCurrentStateJustBeforeTheEffector`,
  `TestTheInterlockIsCheckedAgainAtTheMomentTheEffectorAccepts`,
  `TestATrippedInterlockFailsTheCommandBeforeTheEffector`, domain
  `TestAuthorizationRefusesEachStaleOrAlteredRecord`.

## Open items
- Observer double count: an abandoned lease calls `ObserveLeaseExpiry` twice, once in
  `abandonExpiredLease` and again in `resultAtStanding` while finalizing it, so the lease
  expiry metric counts such a crash twice. The tests assert "observed", not "exactly once",
  so they do not pin it. Decision for the owner.
- A command refused before the effector (stale authorization, tripped interlock) is stored
  `failed` with a verification row left `awaiting`; the domain table pins this
  (`ClassifyDispatch` failure row). Nothing counts it as unresolved, but the row never moves.
  Confirm it is intended.
- `MarkCommandDispatching` moves `pending`, `failed` and `dispatching` commands to
  `dispatching`; the tests pin that settled and unknown commands are never moved, not that a
  `failed` command is revived.
- The ledger fixture exists twice (`app` and `store` test packages): sharing it needs a
  test-support package, and every new package edits the architecture gate tables
  (`import_rules_test.go`, `package_layers_test.go`), outside this round's modules.
- Remaining uncovered code is database-failure wrapping and `dispatchContext`'s non-positive
  lease branch.
