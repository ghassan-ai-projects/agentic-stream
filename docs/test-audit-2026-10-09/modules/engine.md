# engine

Status: done
Round: 5

Audited in round 5 with [eventlog](eventlog.md). Production code is unchanged.

## Metrics

Times are `go test -short -race -count=1` wall time per package, measured while
other workers were running. Tests are top-level tests / passing subtests.

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/engine` (facade) | 78.6% | 100.0% | 1.6-1.8 s | 1.7 s | 1 / 0 | 4 / 4 |
| `internal/engine/internal/app` | 80.2% | 87.8% | 3.1-4.3 s | 3.2 s | 11 / 6 | 28 / 15 |
| `internal/engine/internal/domain` | 92.6% | 95.2% | 1.4 s | 1.3 s | 18 / 10 | 22 / 13 |
| `internal/engine/internal/store` | 72.7% | 93.5% | 1.8-2.2 s | 3.0 s | 11 / 3 | 36 / 23 |

Slowest tests after: `TestDurableProcessingTimerFiresExactlyOnceAcrossARestart`
1.0 s, `TestGlobalRunResumesAfterTheAppliedPosition` 0.9 s. None is near 5 s.
`TestEngineRetriesApplyAfterTransientSQLiteBusy` waited 100 ms on a timer; its
replacement waits on nothing (below).

Hygiene lint (paralleltest, tparallel, usetesting, thelper): 9 findings in
the module before, 0 after. Repository `golangci-lint`: 0 issues, also at
`dupl` threshold 60 (the "known duplication" list in AGENTS.md is untouched).
`time.Sleep` and `t.Skip`: none. `go test -race -shuffle=on -count=3`: passes.
`go test ./internal/architecture/...`: passes.

## Findings and changes

### Removed
- `engine_test.go` (app, 474 lines), dissolved by subject into `run_test.go`, `restart_test.go`, `timers_test.go`, `contention_test.go`, `fixture_test.go`.
- `TestEngineAdvancesCheckpoint`: compiled the 200-line predictive-maintenance spec to prove one event applies once; app tests use the small `restartSpec`. Replaced by `TestRunGlobalAppliesAnEventOnceAndAdvancesTheCheckpoint`, which also asserts the checkpoint position and inbox row (it asserted only the counts).
- `store_test.go` (store, 320 lines), split into six files by subject.
- `TestStoreRequiresTheDatabaseAndOwnerCheck`'s three-expression boolean became a named table (five rows, the old three plus "neither" and the zero value).
- The `appendObservedEvent`/`appendLevel`/`appendHeartbeatWithBoot` fixtures with a `ctx` parameter next to `t`: replaced by `thingEnvelope` and `appendEnvelope` using `t.Context()`.

### Renamed or moved
- `app/helpers_test.go` → `fixture_test.go` (holds every fixture and spec).
- `app/fault_test.go` → `rollback_test.go`; `TestRecordWriteFailureRollsBackAndTheEventAppliesOnceAfterRecovery` kept (six fault points).
- `domain/heartbeat_pin_test.go` → `timer_identity_test.go` (names the identities and instant texts it pins).
- App tests: `TestEngineRestoresSituationStateAcrossRestart` (one 80-line test with four phases) → `TestRestartRestoresSituationIdentityAndContinuesItsVersions`, `TestRestartKeepsReducerStateThatPublishesNoNewVersion`, `TestRestartRefusesRuntimeStateItCannotTrust` (table: legacy codec, unknown codec, tampered state). `TestEngineFiresDurableProcessingTimerExactlyOnce` → `TestDurableProcessingTimerFiresExactlyOnceAcrossARestart` plus `TestMissingHeartbeatMakesTheSituationUncertainUntilTheHeartbeatReturns`. `TestEngineRetiresTimerFromPreviousDeviceBoot` → `TestTimerOfAPreviousDeviceBootIsRetiredWithoutFiring`. `TestEngineCreatesTriggerAndSchedulerItem` → `TestANewSituationVersionReachesCognitionInTheSameTransactionWhenEnabled`.
- Store: `TestStoreRequiresTheDatabaseAndOwnerCheck`, `TestOwnerFailureRefusesTheUnitOfWork`, `TestRecordedEventsAreIdempotentAndAdvanceTheCheckpoint`, `TestFailedUnitOfWorkRollsBackWholeAndWALCheckpointSucceeds` → `checkpoint_test.go`; operator state → `operator_state_test.go`; situations and versions → `situations_test.go`; timers → `timers_test.go`.

### Improved
- T5: `TestEngineRetriesApplyAfterTransientSQLiteBusy` released a held write lock after a 100 ms timer and a 5 s `time.After` guard. `TestRecordIsRetriedAfterTransientSQLiteBusy` makes the first attempt fail with a real `SQLITE_BUSY` deterministically: inside the engine's transaction (which holds the write lock) the owner check asks a second connection for the same lock; the second attempt succeeds. It asserts exactly two attempts and one applied event, and needs no `MaxOpenConns`/`busy_timeout` tuning of the engine's own connection.
- T6: all tests and subtests parallel. The old file-database tests used `defer db.Close()`, shared nothing, but were not parallel.
- T4: restart test used `ExecContext` to bump a codec version back and forth; now one test per refusal with `requires rebuild`, `unsupported state codec 9`, `digest mismatch` asserted. The old test "restored situation advances to version 2" never proved it: the situation was already at version 2 after the first event (the example spec chains two transitions). The new test uses `escalationSpec` (watch at level > 10, warning at level > 50) and asserts the restored situation goes from version 1 to version 2 with `previous_version = 1` and the same situation id.
- T4: `TestRunStopsWithoutRuntimeOwnershipAndAppliesNothing` checked only the inbox; it now checks inbox, situations and checkpoints.
- T9: `restartSpec`, `heartbeatSpec`, `escalationSpec`, `thingEnvelope`, `newRig`, `countRows`, `queryText`, `openDatabaseFile`, `reopenEngine` replace per-test setup. The compiled example spec is built once per store package (`sync.OnceValues`). The store tests share `publish`, `failingTx` and `checkTxFailures`.
- Domain: `HeartbeatTimers` stability check compared one ID with either of two (always true for map order); it now compares per state key. `hexOf` re-implemented `hex.EncodeToString`.

### Added
Facade (78.6% → 100%): `TestNewRefusesEveryMissingSafetyDependency` now asserts which refusal; `TestEngineAppliesEvidenceAndExposesItsSituationsThroughTheFacade` (`ListSituations` and `SituationVersion` were 0%), `TestReplayOwnershipAssertsNothing` (0%), `TestEngineUsesTheDefaultTenantUnlessAnotherIsConfigured`.

App (80.2% → 87.8%): `TestLateEventIsAppliedButNeverPullsThePartitionWatermarkBack`, `TestAnEventAlreadyInTheInboxChangesNoState`, `TestConcurrentRunsApplyEachEventExactlyOnce` (four goroutines under `-race`), `TestTheSameEvidenceYieldsIdenticalSituationsOnEveryRun` (two databases, ids, digests, lineage, timestamps), `TestRolledBackRecordLeavesNoStaleInMemorySituation`, `TestRestartRefusesRuntimeStateItCannotTrust`, `TestTimerFailureRollsBackTheFiringAndTheTimerStaysPending`, `TestTimerDerivedVersionReachesCognitionInTheFiringTransaction`, `TestCognitionIsNotReachedWhenDisabled`, `TestNewNamesAnInvalidSpecInsteadOfBuildingPlanes`, `TestNewRequiresSchemaValidationOnTheLogWhenEveryInputDeclaresASchema`, `TestRunGlobalNamesTheStepThatFailed`, `TestInvalidHeartbeatDurationFailsTheWholeRecord`, `TestPublishedVersionsAreReadableByNumberAndAsTheCurrentOne`, `TestReadsNameTheTenantAndSituationTheyFailedFor` (app `ListSituations`/`SituationVersion` were 0%).

Domain (92.6% → 95.2%): `TestRestoreRefusesAFactTimeItCannotParse`, `TestRestoreKeepsFactsThatAreNotEventTimes`, `TestRestoreGivesASituationWithoutFactsAnEmptyFactMap`, three more identity-mismatch rows and the not-a-JSON-document row, `TestTimerWithoutTheExpectedLastEventIsNotMatched`.

Store (72.7% → 93.5%): `TestOwnerCheckReceivesTheEpochAndTheOpenTransaction`, `TestCheckpointAdvancesPerPartitionAndAppliedThroughIsTheGreatestPosition` (`AppliedThrough` was 0%), `TestInboxAndCheckpointAreScopedToTheTenant`, `TestRetryBusyRunsTheWorkOnceWhenItSucceeds`, `TestStoreReadsNameTheirFailureOnAClosedDatabase`, `TestCheckpointStatementsNameTheirFailure`, `TestOperatorStateIsScopedToPartitionAndTenant`, `TestSavingNilOperatorStateLeavesStorageUntouched`, `TestStoredOperatorStateRoundTripsItsBlob`, `TestCorruptOperatorStateRefusesTheLoadInEitherScope`, `TestOperatorStateStatementsNameTheirFailure`, `TestRuntimeStateOfADivergedVersionIsRefused`, `TestAPublishedSituationVersionIsNeverRewritten`, `TestInsertedVersionRecordsItsPredecessorOnlyWhenItHasOne`, `TestEachCurrentSituationRestoresOnlyTheDeploymentsTenantInEntityOrder`, `TestSituationStatementsNameTheirFailure`, `TestListSituationsIsTenantScopedFilterableAndNewestEvidenceFirst`, `TestSituationVersionReadsAnExplicitVersionOrTheCurrentOneWithItsEvidence`, `TestSituationVersionRefusesUnknownTenantSituationAndVersion`, `TestSituationVersionRefusesUndecodableLineageReferences`, `TestListSituationsNamesAQueryFailure` (the whole `Reader` was 0%), `TestArmingANewHeartbeatTimerCancelsThePendingOneOfTheSameStateKey`, `TestDueTimersAreOrderedByDueTimeAndScopedToPartitionAndTenant`, `TestTimerStatementsNameTheirFailure`, `TestProcessVersionHandsTheVersionAndTheOpenTransactionToTheProcessor` (0%).

### Speed
- The module was already fast. Removing the repeated compile of the example spec (once per store package) and the 100 ms busy wait saved about 0.2 s per run; the added tests cost more than that, so package time is flat to slightly higher in the store (more tests) and lower in app.

## Production code touched
- none.

## Invariants proven here
- 2 (event time, watermark, late data explicit): `TestWatermarkTrailsEventTimeAndNeverMovesBackwards`, `TestTimerWatermarkFallsBackToNowBeforeTheFirstRecord` (domain); `TestLateEventIsAppliedButNeverPullsThePartitionWatermarkBack` (app: the late record is applied and logged in the inbox, the checkpoint watermark holds, the newer fact is kept). Mutation check: removing the monotonic clamp fails the domain and app tests.
- 3 (Situation version immutable after publication): `TestAPublishedSituationVersionIsNeverRewritten` (store: a second insert of version 1 is refused and later publishes and runtime-state writes leave its snapshot untouched; a mutation to upsert on conflict fails it); `TestRuntimeStateOfADivergedVersionIsRefused`; `TestPublishedVersionsAreReadableByNumberAndAsTheCurrentOne` (app).
- 4 (deterministic state changes serial per partition): `TestConcurrentRunsApplyEachEventExactlyOnce` (app, under `-race`; removing the run lock fails it); `TestTheSameEvidenceYieldsIdenticalSituationsOnEveryRun` (determinism of ids and digests).
- 8 (inbox, outbox, idempotency): `TestRecordedEventsAreIdempotentAndAdvanceTheCheckpoint`, `TestInboxAndCheckpointAreScopedToTheTenant` (store); `TestAnEventAlreadyInTheInboxChangesNoState`, `TestGlobalRunFailurePreservesProgressAndInboxDeduplication`, `TestGlobalRunResumesAfterTheAppliedPosition`, `TestRecordWriteFailureRollsBackAndTheEventAppliesOnceAfterRecovery`, `TestRolledBackRecordLeavesNoStaleInMemorySituation`, `TestRecordIsRetriedAfterTransientSQLiteBusy`, `TestDurableProcessingTimerFiresExactlyOnceAcrossARestart`, `TestHeartbeatTimersAreReplacedAndAcknowledgedOnce` (store: a fired timer is never revived).
- 2 (completeness explicit): `TestMissingHeartbeatMakesTheSituationUncertainUntilTheHeartbeatReturns`.

## Open items
- `RunGlobal` counts an event already present in the inbox as processed (the record transaction returns without recording anything). `TestAnEventAlreadyInTheInboxChangesNoState` pins that no state changes, not the count. The log suppresses duplicate ids at append, so the case needs a hand-made inbox row; whether the count should exclude it is a behavior question.
- The remaining uncovered app code (87.8%) is error wrapping around `restoreAfterRollback` failing during a rollback, cognition construction failure, and `ApplyTimer` operator errors. Each needs a collaborator that fails only there; not worth a fake.
- A single event can chain several phase transitions (the example spec moves candidate → watch → warning on one event) and only the last version is published, so version numbers can start at 2. That is the situations module's behavior (round 6); the engine tests avoid asserting that the first version is 1 where the spec chains.
