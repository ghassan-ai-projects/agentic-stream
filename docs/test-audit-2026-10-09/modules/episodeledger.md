# episodeledger

Status: done
Round: 8

Audited in round 8 with [approvalledger](approvalledger.md). Production code is
unchanged. Every transition of the queue item, the episode and the fenced
attempt now has a named test at the layer that owns it, every refusal (wrong
state, stale fence, owner lost, expired, already left pending) has one, and
crash recovery is proven from the store up to the facade.

## Metrics

Times are `go test -short -race -count=1` wall time per package, measured while
other workers were running. Tests are top-level tests / passing subtests.

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/episodeledger` (facade) | 64.5% | 100.0% | 2.3 s | 1.6 s | 19 / 8 | 11 / 0 |
| `internal/episodeledger/internal/app` | 71.1% | 88.4% | 1.7 s | 3.0 s | 12 / 12 | 53 / 39 |
| `internal/episodeledger/internal/domain` | 92.7% | 95.5% | 1.2 s | 1.2 s | 19 / 3 | 31 / 26 |
| `internal/episodeledger/internal/store` | 67.5% | 86.1% | 1.5 s | 2.5 s | 10 / 0 | 47 / 8 |

App and store got slower because they hold 4 to 5 times more tests, each on its
own migrated temporary database (about 25 ms under `-race`). No test is above
0.6 s; no package is above 3 s. The facade got faster: 19 tests that each opened
a database and walked a whole scenario became 11.

Hygiene lint (paralleltest, tparallel, usetesting, thelper): 22 findings before,
0 after. Repository `golangci-lint`: 0 issues. `dupl` at threshold 60: 0 in this
module. `time.Sleep`, `time.Now()` and `t.Skip` in tests: none. Time is a fixed
`now` or an offset from it, passed to every operation (the ledger never reads a
clock). `go test -race -shuffle=on -count=3`: passes. `go test ./internal/architecture/...`: passes.

Mutation checks (production line changed, tests run, line restored): removing
the live-lifecycle filter of `SupersedeEpochEpisodes`, making `Canceling()`
false, swapping the owner-lease and attempt-state checks, changing the expiry
comparison to `Before`, and changing the due-window comparison each fail named
tests. Making `InsertRejection` update on conflict survives: the rejection id
is a hash of reason, details and instant, so a conflict can only be the same
row (equivalent mutant).

## Findings and changes

### Removed
- `cancellation_fence_test.go` (facade): its three rows live where the rule is,
  in `internal/app/cancellation_test.go`, now over every target status.
- `scheduler_expiry_test.go` (facade, 131 lines) and most of
  `scheduler_lifecycle_test.go` (facade): queue ordering, expiry and coalescing
  are app and store rules (T2). The facade keeps one test per family.
- `TestTerminalAndClosedStates` (domain): the terminal and closed sets are pinned
  by `status_sets_test.go`; `KindOf` and `MayAcknowledgeCancellation` got their own
  exhaustive tests.
- `TestSupersessionAndRecoveryWritesReportTheirRows`, the second half
  (store): called five supersession writers and asserted only `err == nil` (T4).
  Replaced by tests that read the rows each writer changes.
- `TestEpisodeLifecycleWritersAndSupersession` (app): eight writers in map order
  asserting no error and the last lifecycle (T4). Replaced by writer-by-writer
  assertions in store and use-case tests in app.
- `seedEpisode` hacks that turned foreign keys off inside tests that did not
  need it (`TestUnknownWorkerRejection...`, `TestCancellationRecovery...`): the
  app and store tests use `OpenTempWithoutForeignKeys` by design; the facade
  keeps foreign keys on for the fencing, owner and recovery tests.

### Renamed or moved
Facade (every old test and where it lives now):

| Old | New |
| --- | --- |
| `lifecycle_test.go` / `TestFencingRejectsLateOutputWithIdenticalSnapshot` | `fencing_test.go` / `TestALateOutputOfAnEarlierAttemptIsRefusedAndAuditedAfterARetry`; closed-episode and unknown-episode steps are `TestAWorkerIdentityIsRefusedWithTheReasonItBreaks/closed_episode` and `TestARejectionOfAnUnknownEpisodeIsAuditedWithoutAForeignKey` (app) |
| `TestRecoveryAbandonsPriorEpochAndIsIdempotent` | `TestCrashRecoveryAbandonsThePreviousEpochsAttemptAndIsIdempotent` (facade) plus the app recovery tests |
| `ownership_test.go` / `TestEpisodeMutationsRemainInsideCallerTransaction` | `transaction_test.go` / `TestEpisodeMutationsRemainInsideTheCallersTransaction`; new `TestAttemptStartTransitionAndRecoveryRollBackWithTheCallersTransaction` |
| `TestAdmissionStoresDeclaredPolicyAndRejectsConflictingLiveEpisode` | `TestAdmissionColumnsRoundTripEveryField`, `TestAnAdmittedEpisodeStartsAdmittedAtTheNanosecondItWasAccepted` (store); `TestASituationHasOneLiveEpisodeAndOnlyAReconsiderationReportsTheConflict` (app); `TestAReconsiderationCollidingWithALiveEpisodeReportsTheConflictThroughTheFacade` |
| `TestAdmissionRefusesAnUndeclaredDispatchPolicy` | `TestAnEpisodeWithoutADeclaredDispatchPolicyIsRefused` (store, asserts the `dispatch_policy` CHECK, not just an error) |
| `TestCancellationRecoveryAbandonsRatherThanRequeues` | `TestRecoveryAbandonsAnEpisodeWhoseAttemptWasBeingCancelledAndReleasesItsCost` (app) |
| `TestUnknownWorkerRejectionIsDurableAndIdempotent` | `TestARejectionOfAnUnknownEpisodeIsAuditedWithoutAForeignKey`, `TestRepeatingARejectionChangesNothingAndANewInstantIsANewRejection`, `TestAnUnregisteredRejectionReasonIsRefusedBeforeAnythingIsWritten` (app) |
| `TestSupersededAttemptOnlyAcceptsCurrentCancellation` | `TestASupersededEpisodeAcknowledgesCancellationButNoOtherOutcome`, `TestOnlyTheCurrentAttemptMayAcknowledgeCancellationOfAClosedEpisode` (app) |
| `TestExpiredSchedulerItemsLeaveThePendingQueue` | `TestExpiredItemsLeaveThePendingQueueAndAreNotPolledAgain` (app) |
| `TestSchedulerItemExpiresExactlyAtItsExpiryInstant` | `TestAnItemExpiresExactlyAtItsExpiryInstant` (app; the pure boundary stays in domain `TestPollExpiresAnItemAtItsExpiryInstantAndNotOneNanosecondBefore`) |
| `TestExpiringANonPendingSchedulerItemIsRefused` | `TestAPendingItemLeavesTheQueueOnlyOnceAndNoOtherTransitionRevivesIt` (app, 4 x 4 matrix) |
| `TestUnreadableSchedulerTimeIsolatesOnlyThatRow` | app `TestAnUnreadableSchedulerTimeIsolatesOnlyThatRow` and store `TestThePendingQueueIsTenantScopedAndIsolatesUnreadableTimes` |
| `TestQueueAdmissionAndCoalescingShareCallerTransaction` | `TestQueueAdmissionAndCoalescingShareTheCallersTransaction` (facade) |
| `TestUpsertPreservesDeterministicItemIdentityAcrossConflicts` | `TestAnItemIdClashKeepsTheExistingItemAndATriggerKeepsItsFirstId` (app), `TestAQueuedItemKeepsItsIdentityWhenItsTriggerIsQueuedAgain`, `TestAnItemIdClashIsClassifiedAndInsertIfAbsentKeepsTheExistingItem` (store) |
| `TestSkippedOpportunitiesCoalesceOnlyWhilePending` | the app matrix above |
| `TestCoalesceReturnsExactlyTheItemsItFlipped` | `TestCoalescingATriggerFlipsOnlyItsOpenItemsAndReturnsThemInIdOrder` (store) and `TestCoalescingATriggerFlipsItsOpenItemsAndReportsTheirVersions` (app) |
| `TestLiveAdmissionOrderEqualsTheReplaySelection`, `TestLiveAdmissionSkipsItemsPastTheirExpiry` | same names, in `internal/app/scheduler_queue_test.go` |
| `TestStatusEnumsMatchTheSchemaChecks` | same (made parallel) |

Other layers:
- `app/app_test.go` → `app/fixture_test.go` (helpers only) plus `attempt_test.go`,
  `identity_test.go`, `cancellation_test.go`, `rejection_test.go`,
  `recovery_test.go`, `scheduler_queue_test.go`, `episode_lifecycle_test.go`,
  `admission_test.go`, `reads_test.go`. `TestAttemptLifecycleRunsUnderFencing`
  (one 25-line test with six unrelated asserts) is `attempt_test.go`.
  `TestCancellationAcknowledgementOfAClosedEpisodeIsStillFenced` moved from
  `owner_fence_test.go` to `cancellation_test.go` as `...ByTheOwner`.
- `domain/rules_test.go` → `fence_test.go`, `rejection_test.go`,
  `recovery_test.go`, `admission_test.go`, `attempt_transition_test.go`;
  `TestSchedulerRules` is `TestAPendingOnlyTransitionMustChangeExactlyOneItem`
  in `queue_test.go`.
- `store/store_test.go` → `fixture_test.go` plus `attempt_test.go`,
  `episode_test.go`, `fence_test.go`, `recovery_test.go`, `rejection_test.go`,
  `scheduler_queue_test.go`, `supersession_test.go`, `dispatch_test.go`;
  `scheduling_reads_test.go` → `scheduling_test.go`.

### Improved
- T6: every test and subtest in the module is parallel (22 findings to 0). The
  facade previously shared nothing but ran serially.
- T4: error tests assert which refusal: `IsIdentityReason` with the exact reason,
  `errors.Is(..., ErrLiveEpisodeConflict)`, the message "scheduler item X is no
  longer pending", "invalid attempt transition dispatched -> produced", the
  `dispatch_policy` CHECK. Writer tests read the rows they changed instead of
  asserting `err == nil`.
- T2: ordering, expiry and coalescing moved from the facade to app and store;
  the facade keeps wiring, transaction joins, rollback and the audit journey.
- T5: expiry tests pass a fixed instant (`now`, `minutes(n)`) to every call and
  assert the nanosecond before and at the boundary; nothing reads a clock.
- T9: `thelper`-clean shared helpers; one `within` per package passes both the
  opaque `*store.Tx` and the raw `*sql.Tx` for seeding.

### Added
Domain (92.7% to 95.5%): `TestAnAttemptMovesOnlyAlongTheTransitionTable` (all 81
state pairs; the old test sampled 8), `TestTerminalAttemptsAcceptNoTransition`,
`TestARefusedTransitionNamesBothStates`, `TestATransitionWritesTheColumnsOfItsKind`,
`TestOnlyCancellationAndAbandonmentAcknowledgeACancelledEpisode`,
`TestEveryRegisteredRejectionReasonIsAccepted` (20 reasons),
`TestAnUnregisteredRejectionReasonIsRefusedByName`,
`TestRejectionIDDependsOnEveryInputAndIsStable`,
`TestAnIdentityRefusalNamesItsReasonAndMatchesOnlyThatReason` (`IdentityError.Error` was 0%),
`TestCheckIdentityIgnoresTheLifecycle`, `TestEveryTerminalAttemptStatusIsRefusedAsTerminal`,
`TestOnlyALiveEpisodeIsStartable`, `TestANewAttemptWaitsForTheActivePriorAttempt`,
`TestOnlyACancellingAttemptAbandonsItsEpisodeOnRecovery`, `TestARecoveredEpisodeIsRequeuedOnlyWhileLive`.

Store (67.5% to 86.1%): the whole read side of `Episode` (0%), `Rejections` ordering,
`Scheduling` with an episode and refusals, `TestAHandleWithoutADatabaseReportsItselfClosed`;
fenced attempt writes (`TestEveryAttemptWriteIsFencedByTheAttemptsEpisodeAndFence`),
fence and attempt id uniqueness (`TestTwoAttemptsCannotShareAnIdOrAFenceOfOneEpisode`),
`TestAKilledEpochSupersedesOnlyItsLiveEpisodes`,
`TestCoalescedItemsSupersedeTheirLiveEpisodesAndCancelTheirInFlightAttempts`,
`TestUnfinishedAttemptsAreTheActiveOnesOfAnyOtherEpoch`,
`TestAbandoningAnUnfinishedAttemptRecordsTheTerminalOnceAndOnlyWhileUnfinished`,
`TestAbandoningAnOpenEpisodeLeavesAClosedEpisodeAlone`,
`TestACostSettlementFailureNamesTheEpisodeAndKeepsItsCause`,
`TestTheOwnerCheckRunsOnTheCallersTransactionAndItsFailureIsWrapped`,
`TestSupersededEpisodesWithoutAnAttemptUnderAKilledEpochAreDispatchableOnlyOnRequest`,
`TestEpisodeIdentityAndTheLiveEpisodeRuleAreEnforcedByTheDatabase`.

App (71.1% to 88.4%): `TestAnEpisodeAllowsOnlyOneActiveAttempt` (3 active states),
`TestAnAttemptStartsOnlyOnAKnownLiveEpisode` (every closed lifecycle),
`TestATerminalAttemptAcceptsNoFurtherTransition`, `TestATransitionRecordsItsTimesAndTerminalDocument`,
`TestALateAttemptCannotTransitionAfterARetryStartedANewFence`,
`TestAWorkerIdentityIsRefusedWithTheReasonItBreaks` (8 rows),
`TestAnOwnedIdentityIsStaleWhenItsAttemptBelongsToAnotherEpoch`,
`TestTheOwnerLeaseIsJudgedBeforeTheAttemptStateAndAfterTheEpisodeFence` (the documented
fence order, mutation-checked), `TestAnEpisodePointingAtAMissingAttemptRowRefusesEveryIdentityAsTheWrongAttempt`,
`TestACancelledAttemptCannotAcknowledgeTwice`,
`TestRecoveryRequeuesAnEpisodeWhoseAttemptBelongedToAnOlderEpoch` (next attempt gets fence 2, old output is stale),
`TestRecoveryLeavesTheCurrentEpochAndFinishedAttemptsAlone`, `TestRecoveryIsIdempotent`,
`TestARecoveryCostFailureNamesTheEpisodeAndFailsTheRecovery`,
`TestRecoveryWithoutACostSettlerStillAbandonsTheCancellingEpisode`,
`TestAKilledEpochSupersedesItsLiveEpisodesSoTheirAttemptsCanOnlyBeCancelled`,
`TestCoalescingSupersedesTheEpisodeAndAsksItsInFlightAttemptToCancel`,
`TestTheLedgerReadsAnEpisodesFenceStatusLifecycleAndAdmission`,
`TestTheDispatcherSeesTheOldestLiveEpisodeAndSupersededOnesOnlyOnRequest`,
`TestTheQueueOfOneTenantIsInvisibleToAnother`,
`TestAnOpportunityIsExplainableFromDurableRecordsWhetherCoalescedExpiredOrRefused`.

Facade (64.5% to 100%): `TestAnEpisodeIsAdmittedRunAndExplainedThroughTheFacade`
(admit, dispatch, owned attempt, produce, refuse, conclude, then every read:
`ReadEpisodeFence`, `ReadAttemptStatus`, `ReadEpisodeLifecycle`, `ReadAdmission`,
`NextDispatchableEpisode`, `Scheduling`, `Episode`, `IsTerminalAttempt`),
`TestTheSchedulerQueueRunsEveryOperationThroughTheFacade`,
`TestSupersessionCancelsLiveEpisodesOfAKilledEpochAndOfCoalescedItems`
(`SupersedeEpoch`, `SupersedeCoalesced` were 0%),
`TestAnOwnerCheckThatReportsALostEpochRefusesTheAttemptAsStale` (`StartAttemptOwned`
was 0%, `ErrOwnerLost` wrapped through the public alias).

### Speed
- Nothing was slow. Per-test cost is the migrated-template copy; all tests are
  parallel, so wall time per package tracks the slowest few tests.

## Production code touched
- none.

## Invariants proven here
- 8 (stable identities, idempotency):
  - attempt identity and fence: `TestStartingAnAttemptAllocatesTheNextFenceAndMarksTheEpisodeRunning`,
    `TestTwoAttemptsCannotShareAnIdOrAFenceOfOneEpisode` (store),
    `TestRecoveryRequeuesAnEpisodeWhoseAttemptBelongedToAnOlderEpoch` (restart keeps the fence, next attempt is fence+1),
    `TestALateOutputOfAnEarlierAttemptIsRefusedAndAuditedAfterARetry` (facade);
  - scheduler item identity: `TestAnItemIdClashKeepsTheExistingItemAndATriggerKeepsItsFirstId`,
    `TestAQueuedItemKeepsItsIdentityWhenItsTriggerIsQueuedAgain`,
    `TestAnItemIdClashIsClassifiedAndInsertIfAbsentKeepsTheExistingItem`;
  - idempotent audit and recovery: `TestRepeatingARejectionChangesNothingAndANewInstantIsANewRejection`,
    `TestARejectionIsRecordedOnceAndKeepsItsReference` (store), `TestRecoveryIsIdempotent`,
    `TestCrashRecoveryAbandonsThePreviousEpochsAttemptAndIsIdempotent`;
  - one live episode per Situation: `TestEpisodeIdentityAndTheLiveEpisodeRuleAreEnforcedByTheDatabase`,
    `TestASituationHasOneLiveEpisodeAndOnlyAReconsiderationReportsTheConflict`;
  - caller's transaction: `TestEpisodeMutationsRemainInsideTheCallersTransaction`,
    `TestAttemptStartTransitionAndRecoveryRollBackWithTheCallersTransaction`,
    `TestQueueAdmissionAndCoalescingShareTheCallersTransaction`.
- 10 (every rejected, canceled, expired opportunity explainable):
  - rejected: `TestARejectionOfAKnownEpisodeIsAuditedWithItsAttemptAndFence`,
    `TestARejectionOfAnUnknownEpisodeIsAuditedWithoutAForeignKey`,
    `TestAnUnregisteredRejectionReasonIsRefusedBeforeAnythingIsWritten`,
    `TestEveryRegisteredRejectionReasonIsAccepted`, `TestAnEpisodesRejectionsReadOldestFirstThenById`;
  - canceled and superseded: `TestAKilledEpochSupersedesOnlyItsLiveEpisodes`,
    `TestCoalescedItemsSupersedeTheirLiveEpisodesAndCancelTheirInFlightAttempts`,
    `TestASupersededEpisodeAcknowledgesCancellationButNoOtherOutcome`,
    `TestRecoveryAbandonsAnEpisodeWhoseAttemptWasBeingCancelledAndReleasesItsCost`;
  - expired: `TestExpiredItemsLeaveThePendingQueueAndAreNotPolledAgain`,
    `TestAnItemExpiresExactlyAtItsExpiryInstant`, `TestAnUnreadableSchedulerTimeIsolatesOnlyThatRow`;
  - explained: `TestAnOpportunityIsExplainableFromDurableRecordsWhetherCoalescedExpiredOrRefused` (app),
    `TestSchedulingExplainsWhatBecameOfAnItemItsEpisodeAndTheRefusedResults` (store),
    `TestAnEpisodeIsAdmittedRunAndExplainedThroughTheFacade` (facade).

Refused transitions, by layer: attempt state machine `TestAnAttemptMovesOnlyAlongTheTransitionTable`
(domain), `TestATerminalAttemptAcceptsNoFurtherTransition` (app); stale fence
`TestAWorkerIdentityIsRefusedWithTheReasonItBreaks/older_fence`; owner lost
`TestOwnedIdentityIsFencedOnEveryTransition`, `TestOwnedStartIsFencedByTheOwnerCheck`;
queue item already left pending `TestAPendingItemLeavesTheQueueOnlyOnceAndNoOtherTransitionRevivesIt`.

## Open items
- The episode lifecycle writers (`Conclude`, `Abandon`, `AbandonRebind`, `RetainForRetry`, `Rebind`, `BindRequest`) carry no `lifecycle_status` guard in their SQL: `Abandon` after `Conclude` overwrites the outcome and `RetainForRetry` revives a concluded episode. They rely on callers (the episode runner) to call them from a valid state. The tests call them only from valid states and do not pin the missing guard. Whether the ledger should refuse is a design question (invariant 10: a closed episode's outcome stays explainable).
- `DueSchedulerItems` returns an item whose admission window opened before its expiry even when `now` is already past the expiry (replay iterates all, live admission skips stale ones through `PollSchedulerQueue`). It is documented and pinned (`TestLiveAdmissionSkipsItemsPastTheirExpiry`); noted because the two reads disagree by design.
- Remaining uncovered code (app 88.4%, store 86.1%) is error wrapping around database failures and `persistTransition`'s `rows != 1`, which cannot happen inside one transaction after the identity check. Each would need a failing connection; not worth a fake.
