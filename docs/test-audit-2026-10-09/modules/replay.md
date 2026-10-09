# replay

Status: done
Round: 14 (speed round)

## Metrics

Timings are `go test -short -race -count=1` wall time taken while other workers were running. Passing tests include subtests.

| Package | Coverage before | after | Time before | after | Tests before (top-level / passing) | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/replay` | 60.0% | 100.0% | 31-36 s | 5.4-6.5 s | 20 / 33 | 13 / 21 |
| `internal/replay/internal/app` | 68.1% | 86.2% | 8.2-8.5 s | 2.8-3.4 s | 8 / 10 | 23 / 42 |
| `internal/replay/internal/domain` | 74.6% | 93.3% | 1.4 s | 1.2 s | 27 / 54 | 36 / 75 |
| `internal/replay/internal/store` | 82.1% | 82.1% | 2.0 s | 2.0 s | 7 / 7 | 7 / 7 |
| `internal/replay/internal/transport` | 67.9% | 83.5% | 5.3 s | 3.0-4.0 s | 9 / 9 | 12 / 12 |

Slowest single test before: `TestThermalChamberReplayIsDeterministic` 32.6 s (golden traces 22 s, shadow tests 19 s, all inflated by CPU contention between ~35 parallel cold replays). Slowest after: 3.6 s under load, 1.5 s alone. No test above 5 s.

## Measurement

- One isolated replay under `-race`: 1.55 s with `storage.OpenFresh` (a cold run of the 33 migrations is ~1.4 s), 0.07 s without `-race`. A seeded open (`storagetest.Open`, migrated template copy) plus the whole predictive replay is 0.3 s. So SLOW_TESTS.md cause 1 (cold migrations) was confirmed and was the bulk of the 36 s: ~35 replays x 1.5 s of CPU, all started at once.
- A replay of the 64-event thermal trace is still 1.2 s under `-race` after seeding (70 ms without `-race`). The cpuprofile is dominated by `runtime.pthread_cond_signal` (58%): goroutine wake-ups from `database/sql` per-statement bookkeeping under the race runtime, in `internal/engine` and `internal/storage`, not replay code. Reported under open items.
- Confirmed: `RunNTimes` ran its n replays one after the other. Not a cause by itself once the migrations were gone, but it made `TestThermalChamberReplayIsDeterministic` 3.6 s alone.

## Findings and changes

### Speed
- `internal/replay/internal/transport/database.go`: `OpenIsolatedDatabase` takes its opener from the context (`WithDatabaseOpener`, default `storage.OpenFresh`). Tests pass `storagetest.Open`, so a replay starts from the migrated template. The real opener stays on the production path and is kept in three tests (below). Facade tests reach it through `replay.WithSeededDatabases` in `export_test.go`.
- All facade, app and transport tests that only need a database now use the seeded opener; independent replays inside a test already ran as parallel subtests or tests.
- `RunNTimes` runs its n replays concurrently (production change, below).
- The real-opener full replay moved from the app package (it was the 3 s long pole there) to the facade (`TestRunMigratesAFreshIsolatedDatabaseItself`), which has headroom.

### Removed (T2/T10: proven once at the lowest layer)
- Facade `TestWorkerAwareModesRequireExplicitCapabilities`: identical to the non-deterministic subtests of `TestReplayModesHaveNoCredentialOrEffectorBoundary`.
- Facade `TestReplayModesHaveNoCredentialOrEffectorBoundary`, `TestDeterministicReplayDoesNotInvokeCognition`, `TestWorkerAwareModesUseOnlySuppliedCapabilities`, `TestCounterfactualModeIsRefused`, `TestRecordedReplayRejectsUnverifiableLedgerEntries`, `TestShadowReplayValidatesAnExecutableOpportunity`, `TestShadowReplayPersistsPairedComparisonWithoutEffects`, `TestShadowReplayReportsDecisionDifferences`, `TestShadowReplayRejectsOutOfCatalogIntent`, `TestDeterministicBaselineProducesAValidatedRecommendation`, `TestRecordedReplayValidatesACompleteLedger`: they exercised `app.RunMode` through a test-only `RunMode` alias in `export_test.go`. They moved to `internal/app` (table below). The alias, `DeterministicBaseline` and `NewDeterministicBaseline` exports are deleted from `export_test.go`.
- App `TestShadowPhaseExecutesPairAndRecordsComparison` and `TestRecordedPhaseValidatesCompleteLedger`: strict subsets of the moved facade tests.
- Copy-pasted doubles (`stubBaseline`/`stubShadow`, `testShadowExecutor`/`testBaselineExecutor`, two ledgers, two `alwaysTriggerSpec`): one set in `internal/app/fixtures_test.go`.

### Renamed or moved
| Old | New |
| --- | --- |
| `replay_test.go` `TestGoldenTracesAreDeterministic`, `TestReplayRejectsExistingDatabaseAndSidecars` | `golden_replay_test.go` (same names) |
| facade `TestReplayModesHaveNoCredentialOrEffectorBoundary` (deterministic) | app `TestDeterministicModeEqualsRunAndNeverInvokesCognition`; (recorded, shadow) `TestRunModeFailsClosedBeforeAnyWorkWithoutCapabilities` |
| facade `TestDeterministicReplayDoesNotInvokeCognition` | app `TestDeterministicModeEqualsRunAndNeverInvokesCognition` (now also checks episodes, intents, commands, outbox are empty) |
| facade `TestCounterfactualModeIsRefused` | app `TestRunModeRefusesRemovedAndUnknownModes/counterfactual` |
| facade `TestWorkerAwareModesUseOnlySuppliedCapabilities` | app `TestShadowModeRequiresBothExecutors`, `TestShadowPhasePairsBaselineWithCandidateOnEveryEpisode`, `TestRecordedPhaseAcceptsAnEmptyLedgerWhenNothingIsExecutable` |
| facade `TestShadowReplayValidatesAnExecutableOpportunity` | app `TestShadowPhasePairsBaselineWithCandidateOnEveryEpisode`, `TestShadowPhaseRefusesAnUnusableCandidateAsAFindingNotAnError/malformed_manifest_digest` |
| facade `TestShadowReplayPersistsPairedComparisonWithoutEffects` | app `TestShadowPhasePersistsOnlyComparisonsAndRepeatsByteForByte` |
| facade `TestShadowReplayReportsDecisionDifferences` | app `TestShadowPhaseReportsADecisionDifferenceAsAFinding` |
| facade `TestShadowReplayRejectsOutOfCatalogIntent` | app `TestShadowPhaseRefusesAnUnusableCandidateAsAFindingNotAnError/intent_outside_the_catalog` |
| facade `TestRecordedReplayValidatesACompleteLedger` | app `TestRecordedPhaseValidatesACompleteLedger` |
| facade `TestRecordedReplayRejectsUnverifiableLedgerEntries` | app `TestRecordedPhaseRefusesALedgerThatDoesNotMatchTheReplay` (6 cases) |
| facade `TestDeterministicBaselineProducesAValidatedRecommendation` | app (same name) |
| app `TestRunDeterministicSessionCollectsCanonicalResult` | `TestRunReplaysTheTraceIntoAFreshIsolatedDatabase` |
| app `TestRunNTimesRepeatsDeterministically` | `TestRunNTimesRepeatsDeterministicallyInSeparateDatabases` + `TestRunNTimesRejectsANonPositiveCountAndNamesTheFailedRun` |
| app `app_test.go` | `fixtures_test.go`, `run_test.go`, `shadow_test.go`, `recorded_test.go`, `source_test.go` |

### Improved
- T4: `err != nil` assertions now name the error in facade, app, transport, store and domain tests (existing-database/sidecar rejection, duplicate comparison, capability sets, snapshot verification, baseline, recorded-ledger entries, read-only source). `TestRunModeFailsClosedBeforeAnyWorkWithoutCapabilities` also proves no database is created. `alwaysTriggerSpec` fails when a rewrite no longer matches the fixture (audit A-075 T2).
- T6: `paralleltest` fixed in `internal/domain` (`baseline_test.go`, `rules_test.go`); `thelper` fixed in `source_test.go`; `context.Background()` replaced by `t.Context()`.

### Added
- app: `TestRunRecordedVerifiesAgainstTheSourceRuntimeDatabase`, `TestRunRecordedRefusesASourceThatCannotVouchForTheReplay` (missing source, spec never deployed, unreadable spec, missing decision), `TestRunShadowPairsTheBaselineWithAWorkerOnAUnixSocket` (in-process `workerfake` over a Unix socket, the only end-to-end proof of `RunShadow`, `DialShadowWorker`, `ExecuteShadow`), `TestRunShadowRefusesWhatItCannotCompareBeforeReplaying`, `TestRunWithoutAReadableSpecFailsBeforeAnyEvidenceIsIngested`, `TestShadowPhaseFailsWhenTheDeterministicBaselineFails`, candidate-failure case (`shadow_candidate_failed`), `TestRunModeRefusesMultipleCapabilitySets`.
- facade: `TestRunRecordedVerifiesAgainstTheSourceDatabase`, `TestRunRecordedNamesAMissingSourceDatabase`, `TestRunShadowRefusesARelativeWorkerSocket` (the facade `RunRecorded` and `RunShadow` had 0%), `TestAllHashesEqualDetectsADifferingRun`, `TestRunMigratesAFreshIsolatedDatabaseItself`.
- transport: `TestOpenIsolatedDatabaseUsesTheOpenerOfTheContext` (key in the transport package), `replaytest.TestWithDatabaseOpenerRoutesEveryReplayThroughTheOpener`, `TestDialShadowWorkerRefusesAnythingButAnAbsoluteSocketPath`, `TestShadowWorkerNamesItselfInTheErrorOfAFailedExecution`.
- domain: `ValidateOutput` (every binding failure and the success path), `Clone` independence, `TraceEnvelope`/`AdoptTenant`/`ContractValidEnvelope`, `ValidateRecordedAttempt`, `DecodeRecordedDecision`, `ValidateRecordedSnapshot`.

## Production code touched
- `internal/replay/internal/transport/database.go`: the `DatabaseOpener` type, the exported context key type `DatabaseOpenerKey`, and `OpenIsolatedDatabase` taking its opener from the context, defaulting to `storage.OpenFresh`. The function that installs an opener lives in the new test-support package `internal/replay/replaytest` (`WithDatabaseOpener`), so `make deadcode` finds no production function reachable only from tests. The package is registered in `allowedImports` and `packageLayers` (layer 30, edges to `internal/replay/internal/transport` and `internal/storage`) and in `documentation/architecture/repository-map.md`; it has its own test (`TestWithDatabaseOpenerRoutesEveryReplayThroughTheOpener`). Facade tests reach it through `replay.WithSeededDatabases` in `export_test.go`; app tests call `replaytest` directly. Before/after: a replay session under `-race` 1.55 s to 0.3 s; the `internal/replay` package 31-36 s to 5.4-6.5 s. `internal/storage` untouched.
- `internal/replay/internal/app/run.go`: `repeatReplay` runs the n replays concurrently (`sync.WaitGroup.Go`), each already in its own database path; results keep run order, the byte-exact hash comparison is untouched, and a failure is reported for the lowest run index (`run 0: ...` as before). Before/after, alone, under `-race`: `TestThermalChamberReplayIsDeterministic` 3.6 s to 1.46 s; app `TestRunNTimes...` 0.33 s to 0.22 s; three cold replays through the real opener 4.66 s to 2.09 s (this is the `run --repeat` path, so `cmd` `TestRunRepeatProvesDeterminism` benefits too).
- `internal/replay/README.md` and `documentation/design/replay-and-shadow.md`: test links and coverage figures updated.

## Invariants proven here
- 9 (replay never performs external effects): `TestDeterministicModeEqualsRunAndNeverInvokesCognition` (no episodes, intents, commands, outbox), `TestRunModeFailsClosedBeforeAnyWorkWithoutCapabilities`, `TestShadowPhasePersistsOnlyComparisonsAndRepeatsByteForByte` (intents, commands, outbox stay empty; `EffectsAllowed` false), `TestRecordedPhaseValidatesACompleteLedger`, `TestRunShadowPairsTheBaselineWithAWorkerOnAUnixSocket`; isolation: `TestRunRejectsExistingDatabase`, `TestReplayRejectsExistingDatabaseAndSidecars`, `TestOpenIsolatedDatabaseRequiresFreshPath`, `TestOpenSourceDatabaseIsReadOnly`.
- Acceptance "identical Situation history for repeated deterministic replay": `TestGoldenTracesAreDeterministic` (3 traces x 3 runs), `TestRunNTimesRepeatsDeterministicallyInSeparateDatabases`, `TestThermalChamberReplayIsDeterministic`.
- Acceptance "reproduce the accepted decision with recorded cognition": `TestRecordedPhaseValidatesACompleteLedger`, `TestRecordedPhaseRefusesALedgerThatDoesNotMatchTheReplay`, `TestRunRecordedVerifiesAgainstTheSourceRuntimeDatabase`, `TestRunRecordedRefusesASourceThatCannotVouchForTheReplay`.
- Acceptance "effect-disabled shadow mode": `TestShadowPhase*` in `internal/app/shadow_test.go`.
- No determinism or isolation assertion was weakened; the seeded opener changes only how the empty database gets its schema.

## Open items
- A thermal-trace replay costs 1.2 s under `-race` (70 ms without) because of goroutine wake-ups in `database/sql` and modernc SQLite per statement (`pthread_cond_signal` is 58% of CPU). Fixing it belongs to `internal/engine` or `internal/storage` (for example fewer transactions per event), outside this round.
- One transient failure of `internal/replay/internal/transport` appeared in the first of ~12 timing runs (package reported `fail`, test not captured). It did not reproduce in 6 further full runs, 15 shuffled runs and a 30x run under load. If it returns, suspect the Unix-socket test (`TestShadowWorkerNamesItselfInTheErrorOfAFailedExecution`) first.
- (Resolved: `make docs-check` passes now.) Earlier it reported four broken links to `internal/runtime/internal/app/{dispatch_mode,pipeline_e2e}_test.go` in `documentation/` (`documentation/guides`, `documentation/learn`, and the shadow/mode tests line of `documentation/design/replay-and-shadow.md`). They come from the concurrent `internal/runtime` test rename, not from this round.
