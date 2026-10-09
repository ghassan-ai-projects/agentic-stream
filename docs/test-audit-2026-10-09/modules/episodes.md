# episodes

Status: done
Round: 7

Four packages: the facade `internal/episodes`, and `internal/app`, `internal/domain`, `internal/store`. The aquaculture catalog `testdata/aquaculture_intents.json` and its pin `TestAquacultureIntentCatalogDigestParity` (`e4f86620…`) are untouched; the thermal catalog pin is unchanged and moved next to it.

## Metrics

Measured with `go test -short -race -count=1 -cover -json`, other workers running (times are noisy; package time is mostly race start-up and linking).

| Package | Coverage before | after | Time before | after | Top-level tests before | after | Passing tests and subtests before | after |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `internal/episodes` | 92.3% | 100% | 1.6 s | 1.8 s | 3 | 6 | 8 | 13 |
| `internal/episodes/internal/app` | 63.9% | 84.3% | 3.5 s | 3.6 s (2.9 s on another run) | 33 | 51 | 42 | 78 |
| `internal/episodes/internal/domain` | 88.6% | 96.0% | 1.6 s | 1.7 s | 41 | 50 | 97 | 138 |
| `internal/episodes/internal/store` | 69.3% | 93.0% | 1.8 s | 2.5 s | 10 | 22 | 13 | 47 |

Test files 30 → 38; `_test.go` lines 4 443 → 4 747 (about 36 more top-level tests and 120 more subtests for 7% more lines: the six seed copies became one builder). Test-hygiene lint (`paralleltest`, `tparallel`, `usetesting`, `thelper`): 43 findings → 0. `time.Sleep`: 1 → 0. Repository lint, `dupl` at 60 on the touched files: only the known `LoadSchedulerItem`/`LoadEvaluation` twin pair in the production store remains (left, as asked). `go test -race -shuffle=on -count=3 ./internal/episodes/...` passes; `go test ./internal/architecture/...` passes.

Time did not drop: the store package gained 34 tests, now cheap because the traced replay runs once (below), and the app package gained 36 tests and lost its sleep. All three internal packages stay under 4 s; no test is above 0.7 s.

## Findings and changes

### Removed
- `TestExecutionFailureClassification` (`decision_rules_test.go`): T10, a strict subset of `TestExecutionFailureClassificationSurvivesWrapping`; the survivor now asserts every cause both bare and wrapped.
- `TestAssemblerRequestContainsDelta`, and the field-by-field "non-empty" checks of `TestAssemblerBuildsEpisodeRequest`: T2/T4, folded into one test that checks values (identity, provenance, allowed intents, delta).
- `TestRunnerOrdersEpisodesByChronologicalAcceptedAt`'s hand-written 40-line SQL fixtures, `seedProducedEpisode`, `seedShadowEpisode`, `seedFreshnessEpisode`, `seedRebindEpisode`, the inline retry fixture, `recordingDeclinedExecutor`, `p8BlockingExecutor`, `blockingExecutor` duplicates, `producedCancelingExecutor`, `cancelingExecutor`, `successfulCancelingExecutor`, `lateProducedOutcomeExecutor`, `slowExecutor`, `jsonEqual`: T9, six copies of the same episode/situation/lineage inserts became one `episodeSeed` builder with options.
- `epoch_control_test.go`, `budget_test.go`, `thermal_catalog_test.go`: helper moved into the fixtures file; the ordering test moved to `runner_test.go`; the thermal pin moved to `intent_catalog_parity_test.go` (T3: `budget_test.go` tested dispatch order, not budgets).
- `zeros`, `deepCopy`, `catalogTicketSchema` duplicates and every explanatory comment block in tests (T11): "P4 exit gate 5", "ISSUE-061", "P8 freshness", "(B1)…(B6)", "phase P8". The guidance now lives in test names and failure messages.

### Renamed or moved
- `app/test_config_test.go` → `app/export_test.go` (Go convention; it exports test wiring: `NewRunner`, `NewAssembler`, `With*`; gains `WithCostLedger`, `WithTelemetry`).
- `app/lifecycle_fixture_test.go` → `app/admitted_situation_test.go` (it builds an admitted situation, not a lifecycle).
- New `app/episode_fixtures_test.go`: the seed builder, `scalar[T]`, `executorFunc`, run and wait helpers.
- `assembler_test.go` split: reconsideration and live-episode-conflict tests → `assembler_reconsideration_test.go`; `ISSUE-061` comments dropped.
- `app`: `TestRunnerExecutesAdmittedEpisode` → `TestRunnerExecutesAnAdmittedEpisodeAndGovernsItsDecision`, `TestRunnerNoWorkWhenEmpty` → `TestRunnerReportsNoWorkWhenNothingIsAdmitted`, `TestRebindRaceReproducesWaterM7` → `TestEpisodeAdmittedBeforeTheSituationAdvancedIsReboundToTheLiveVersion`, `TestDispatchRefusesStaleSituation` → `TestDispatchQuarantinesAnEpisodeBoundToAStaleSituationVersion`, `TestDispatchKillCancelsInFlightAndRefusesItsDecision` → `TestRunnerCancelsAnInFlightAttemptWhenItsEpochIsKilled`, `TestRunnerQuarantinesLateOutcomeAfterEpochKill` → `TestRunnerRefusesTheDecisionOfAnAttemptWhoseEpochWasKilledMidFlight`. Epoch tests moved from `cancellation_test.go` to `epoch_refusal_test.go`.
- `domain`: `decision_rules_test.go` → `decision_test.go`, `decision_input_catalog_test.go` → `decision_input_test.go`, `rules_test.go` → `snapshot_test.go` (its budget test went to `request_budget_test.go`), `admission_roundtrip_test.go` → `admission_test.go` (gained the rebind, cost-budget and shadow-default tests from `assembly_test.go`), `execution_failure_test.go` → `failure_test.go` (`TestProducedOutcome…` → `execution_test.go`, `TestEpochRefusal…` → `epoch_test.go`, `TestTerminalAttemptStatus` → `lifecycle_test.go`), `intent_catalog_test.go` → `intent_catalog_parity_test.go` (aquaculture and thermal pins together) with the rule tests in a new `intent_catalog_test.go`.
- `store`: `store_test.go` → `store_fixtures_test.go`; tests split by subject into `projections_test.go`, `decision_test.go`, `lifecycle_test.go`, `transaction_test.go`, `owner_fence_test.go`.

### Improved
- T5: `slowExecutor` slept 50 ms to outlast a 1 ms budget. `TestDispatchRefusesADecisionThatArrivesAfterTheWallTimeBudget` now uses a virtual clock the executor advances, with rows below, at and past the budget (exactly at the budget is accepted, one millisecond over is `timed_out`/`decision_after_deadline`). `admitTriggeredSituation` used `time.Now()` and a physical cognition clock; it now uses a fixed time and a virtual clock.
- T5: goroutine tests used `time.After(time.Second)` guards inline; `receive`/`mustFinish` wait up to 10 s on a channel and name what they waited for.
- T6: 43 hygiene findings fixed; every test and subtest is parallel. `context.Background()` replaced by `t.Context()`. `seedEpisode` toggled `PRAGMA foreign_keys` on whichever pooled connection answered (and called `SetMaxOpenConns(1)`); the builder now holds one `db.Conn` for the whole seed.
- T4: error tests assert which error (`sql.ErrNoRows`, `context.Canceled`, `ErrLiveEpisodeConflict`, `ErrCostReservationRejected`, `*IdentityError` with reason, message prefixes) instead of `err != nil`. `TestAssemblerTenantMismatch` only checked that some error occurred; it now asserts "tenant mismatch" and no request. Shadow tests assert score, reason and correlation to the decision, not only counts.
- T9: the store tests replayed the predictive-maintenance trace for every test (8 replays, now 22 tests would have needed 22). The replay now runs once; each test copies the migrated, replayed database file and opens it.
- T11: giant tests split (`TestAssemblerBuildsEpisodeRequest` mixed identity, payload and three floor cases with a shared mutated spec; now a table with one admitted situation per row). Test bodies longer than 50 lines are gone except the two fixture builders.
- Domain: `TestDecisionInputRejectsCatalogAttacks` asserted only that an error occurred and ran serially; it now asserts the failing stage per attack and the honest payload's bound fields.

### Added
- app, domain, store behaviors that had no test and enforce a rule:
  - `TestRunnerRejectsAnOutcomeThatDoesNotCarryTheAttemptIdentity` (wrong attempt and stale fence, recorded rejection, no decision persisted), `TestRunnerConcludesAnEpisodeAfterThreeRejectedOutcomes`, `TestRunnerFailsAnAttemptWhoseExecutorReturnsNoOutcome`, `TestRunnerRecordsADecisionTheContractRejectsAndNeverGovernsIt`, `TestRunnerRefusesToFinishAnAttemptWithANonTerminalStatus`, `TestRunnerConcludesAnEpisodeAfterThreeFailedAttempts`.
  - Cost control: `TestRunnerSettlesTheReservedCostWithWhatTheWorkerReported`, `TestRunnerReleasesTheReservedCostOfAnEpisodeItGivesUp`, `TestPersistReservesTheEpisodeCostBudgetWhenCostControlIsOn`, `TestPersistRefusesAnEpisodeThatExceedsTheCostCeiling` (atomic: no episode row).
  - Assembly: `TestAssemblerRefusesAnItemWhoseEvidenceIsBroken` (missing scheduler item, evaluation, snapshot, bad snapshot trace context, undecodable delta), `TestPersistRefusesARequestWithoutDecodableProvenance`, `TestAssemblerCarriesTheWatchConfidenceFloor`.
  - Service and facade: `TestNewRefusesAConfigurationThatCannotDispatchSafely`, `TestAssemblyOnlyServiceRefusesToRunEpisodesBeforeTouchingState`, `TestServiceRunsAnAdmittedEpisodeThroughItsConfiguredRunner`, `TestServiceAssemblesAndPersistsOnTheCallersTransaction` (caller rollback leaves no episode and a pending item), `TestDecisionsReadsEveryDecisionOfAnEpisodeInOrder`, `TestDecisionsNamesTheEpisodeWhenTheReadFails`, facade `TestDecisionsReadsWhatTheRuntimeRecordedForAnEpisodeAndNothingElse`, `TestCompileIntentCatalogBindsTheSpecIntentsToTheirDigest`, `TestConstructionAcceptsOwnedAndShadowExecution`.
  - Operational counters: stale rebind, stale rejection and rebind failure each increment exactly their own counter (in the rebind tests).
  - Domain: `TestCompileIntentCatalogRejectsAnInvalidCatalog` (13 rejection rows: empty, no type, duplicate, risk, schema shape, writable and preset fields, preset bounds, rate limit), `TestCompileIntentCatalogCarriesOnlyTheDeclaredOptionalFields`, `TestCompileIntentCatalogDigestBindsTheCatalogOrder`, `TestDecodeRequestDigestsNamesTheFirstUndecodableDigest`, `TestBindAttemptIdentityAddsOnlyTheAttemptAndKeepsTheDocumentCanonical`, `TestRebindRefusesAnEntityChangeBeforeItDecodesTheRequest`, `TestDecisionInputBindsTheAttemptAndTheRequestAuthority`, `TestDecisionInputRefusesAnUndecodableRequest`, `TestStorageDecisionDigestFallsBackToTheRawHash`, `TestHydrationOfARequestWithoutASnapshotLeavesTheEntityUnbound`.
  - Store: `TestDecisionsAreReadInOrdinalOrderWithTheirProvenance`, `TestDecisionsAfterTheFirstTakeTheNextOrdinal`, `TestValidatedIntentWithAnUndecodableDigestIsRefusedBeforeAnyWrite`, `TestLedgerMovesRunThroughTheEpisodeTransaction` (rebind, abandon, abandon-rebind, conclude), `TestFailedAttemptIsRetainedForRetryOnlyAfterItsTerminalTransition`, `TestCostIsReservedAndSettledOnTheEpisodeTransaction`, `TestReadsOfAnUnknownRowReportNoRows`, `TestSupersededEpisodeIsSeenAsSupersededOutsideATransaction`, `TestStoreReportsWhetherItIsConfiguredAndFenced`.

### Speed
- `dispatch_freshness_test.go`: the 50 ms `time.Sleep` (and its 1 ms budget race) is gone; the virtual clock makes the deadline cases exact.
- Store: one trace replay per package instead of one per test (the package grew from 10 to 22 top-level tests). Wall time of the package is dominated by race start-up; the replay runs once for 22 tests (8 replays before).
- No test exceeds 0.7 s; no package exceeds 4 s.

## Production code touched
- none. (`app/export_test.go` replaces a test-only file that already lived in package `app`.)

## Invariants proven here
- 5 (every episode bound to one immutable Situation snapshot and a finite budget):
  - snapshot binding: `TestRequestAssemblyBindsProvenanceAndEvidence`, `TestValidateSnapshotEvidenceRejectsTamperingAndIdentityDrift`, `TestAssemblerBindsTheRequestToTheSituationSnapshotAndTheSpec`, `TestRebindPreservesAdmissionEvidence`, `TestRebindChangesOnlyTheSnapshotFields`, `TestRebindRefusesAnEntityChangeBeforeItDecodesTheRequest`, `TestDispatchQuarantinesAnEpisodeBoundToAStaleSituationVersion`, `TestRebindFailsClosedOnACorruptLiveSnapshotWithoutStallingTheQueue`, `TestStaleEpisodeIsAbandonedOnceItsRebindBudgetIsSpent`.
  - finite budget: `TestWallTimeBudgetValidatesOnceAtRequestBoundary`, `TestParseWallTimeBudgetValidatesBounds`, `TestDispatchNeverRewritesTheAdmittedBudget`, `TestDispatchRefusesADecisionThatArrivesAfterTheWallTimeBudget`, `TestRunnerConcludesAnEpisodeAfterThreeFailedAttempts`, `TestPersistRefusesAnEpisodeThatExceedsTheCostCeiling`, `TestRunnerSettlesTheReservedCostWithWhatTheWorkerReported`.
- 6 (a model can only propose typed intents, never execute effects): `TestDecisionInputFailsClosedOnAnUntrustedIntentCatalog`, `TestCompileIntentCatalogRejectsAnInvalidCatalog`, `TestAquacultureIntentCatalogDigestParity`, `TestThermalIntentCatalogDigestParity`, `TestRunnerRecordsADecisionTheContractRejectsAndNeverGovernsIt`, `TestRunnerRejectsAnOutcomeThatDoesNotCarryTheAttemptIdentity`, `TestShadowDispatchScoresTheDecisionWithoutEnteringGovernance`, `TestOnlyAnExplicitActivePolicyEntersGovernance`, `TestValidatedIntentIsStoredPendingForThePolicyPlane` (accepted intents stay `pending` for policy), `TestRunnerRefusesTheDecisionOfAnAttemptWhoseEpochWasKilledMidFlight`.
- MVP acceptance "cancel a stale episode after material supersession": `TestRunnerCancelsTheStreamedAttemptOfASupersededEpisode` (a superseded episode's in-flight provider call is cancelled, attempt `cancelled`, no decision persisted), `TestRunnerCancelsAnInFlightAttemptWhenItsEpochIsKilled`, `TestStaleEpisodeRebindsToTheLiveVersionAndDispatches`, `TestEpisodeAdmittedBeforeTheSituationAdvancedIsReboundToTheLiveVersion`, `TestRetryBudgetNeverRevivesSupersededEpisode`.
- Deterministic replay input identity: `TestAssemblingTheSameItemTwiceBindsTheSameSnapshot`, `TestRequestAssemblyBindsProvenanceAndEvidence` (byte-identical re-assembly), `TestCompileIntentCatalogDigestBindsTheCatalogOrder`.

## Open items
- `internal/episodes/internal/store/assembler.go` `LoadSchedulerItem`/`LoadEvaluation`: the known structural twins; they cannot be reduced by a change in tests, left as listed in AGENTS.md.
- Finding for the owner, not changed: when an executor returns a decision-less outcome with a non-terminal `Status` (for example `running`), the claim transaction already committed the attempt, `finishAttempt` returns an error and rolls back, and RunOnce returns the error with the attempt left `running` (the test pins this error). Whether such an attempt should fail closed like a nil outcome is a behavior decision.
- Finding for the owner, not changed: with cost control on, `Settle` of an episode that never reserved returns an error from `RunOnce` after the attempt ran, leaving the episode `running` (seen while writing `TestServiceRunsAnAdmittedEpisodeThroughItsConfiguredRunner`). Admission always reserves first, so only a hand-seeded episode reaches it.
- Remaining uncovered statements in `app` are error wraps of ledger calls that fail only on a closed database; `domain` ones are `json.Marshal`/`canonicaljson` failures on fixed shapes. Not tested on purpose.
- Production comments inside `internal/episodes/internal/**` still violate the AGENTS.md no-comments rule (for example "ISSUE-061", "P8" in `runner_claim.go`, `runner_conclude.go`); out of scope for a behavior-preserving test round.
- `documentation/design/cognition.md` and `documentation/architecture/invariants.md` link to `rebind_test.go`, which still exists; the invariant-to-test map is round 17's.
