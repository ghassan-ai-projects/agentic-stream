# cmd/agentic-stream

Status: done
Round: 16 (the speed round)

## Metrics

Measured with `go test -short -race -count=1 -cover -json ./cmd/agentic-stream`, other workers running (machine load 3-9 on 10 cores), so compare tests by their own durations.

| Package | Coverage before | after | Time before | after | Tests before (top-level / passing) | after |
| --- | --- | --- | --- | --- | --- | --- |
| `cmd/agentic-stream` | 74.3% | 83.1% | 51-58 s | 9.6-11.1 s (4 runs; 15.1 s before the replay opener below) | 41 / 75 | 48 / 83 |

Slowest tests, before → after, in the full-package run (alone under `-race`: 4.9 s, 7.6 s, 5.8 s, 1.6 s, 0.2 s, 0.2 s):

| Test | Before | After (in package run) |
| --- | --- | --- |
| `TestExperimentClosedLoopThroughServe` | 39.0 s | 6.1-7.1 s (4.9 s alone) |
| `TestExperimentClosedLoopUnderAContinuousFeed` | 37.2 s | 7.7-8.0 s (7.6 s alone) |
| `TestExperimentInterlockStopsEffects` | 33.1 s | 5.6-6.6 s (5.8 s alone) |
| `TestRunRepeatProvesDeterminism` | 14.0 s | 0.2 s |
| `TestPrincipalsApplyProvisionsTheExampleGovernance` | 8.0 s | 0.9 s |
| `TestInterlockTripAndClear` | 8.0 s | 1.0 s |
| `TestNotificationsPruneEnforcesTheFloorAndCountsFirst` | 8.0 s | 0.9 s |
| `TestCommandsListAndResolveRefuseWhatIsNotAwaiting` | 7.5 s | 0.6 s |
| `TestExperimentShadowReplayComparesTheCandidate` | 3.2 s | 1.7-2.0 s (1.6 s alone) |

The three experiment tests are 26-31 s faster each. They are ingestion-bound (64 trace events through a race-instrumented pipeline, then the 2 s debounce), so they are no faster alone than in the package run; the slowest is 7.6-8.0 s, and all three are in the slow-test register.

Hygiene lint (`paralleltest`, `tparallel`, `usetesting`, `thelper`): 23 findings → 0 (the one `usetesting` finding, `os.MkdirTemp` in `privateSocketDir`, is a `//nolint:usetesting` with the true reason: `t.TempDir()` paths exceed the Unix socket path limit). Repository `golangci-lint`: 0 issues. `go test -race -shuffle=on -count=3 ./cmd/agentic-stream` passes. No `time.Sleep` left in the package.

## Where the 58 s went (measured)

- The three experiment tests waited on the spec's wall-clock gates. Ledger timestamps showed the first Situation trigger queued at t+2.5 s with `not_before` = +5 s (debounce) and the second with `not_before` = first item created + 20 s (cooldown); the episode started exactly at the cooldown, then the worker delay (5 s) and a fixed `time.Sleep(1s)` followed. That was 20 s of the 30 s of the first test and 22-27 s of the other two.
- Each operator command test cold-migrated its `--db` once per CLI invocation (33 migrations, about 1.5-2 s each under `-race`), 4-5 times per test, as SLOW_TESTS.md predicted.
- `run --repeat 3` and the three recorded-replay checks each ran cold replays one after another. (`RunNTimes` is concurrent since round 14; the recorded checks are now parallel subtests.)
- The first `serve` start in each experiment test cold-migrated its database (1.5 s).

## Findings and changes

### Speed
- Experiment tests keep their behavior and assertions; only the test copy of `zone-thermal-sim.situation.yaml` is edited (the example under `examples/` is untouched): `debounce: 5s → 2s`, `cooldown: 20s → 2s` for all three (`shorterCognitionGates`, merged under each run's own `specEdits`). Measured: the gap between the first and the last scheduled trigger of the trace is about 0.35 s (0.31-0.34 s in four runs), so the first trigger is still coalesced into the second before its debounce ends, with a 6x margin. Stress run: three processes of the experiment tests with `-count=3` at once (nine `serve` runs in parallel, on top of the other workers) all pass; so do six shuffled iterations.
- `TestExperimentClosedLoopUnderAContinuousFeed`: `slide: 30s → 1s` (was 2 s), worker delay `5 s → 2 s`: the worker still reasons across at least two window slides (each publishes a provisional and an on-time version), so the property under test, slides landing inside the episode, still holds. The assertion that 155 non-material versions leave exactly one approved intent and one command is unchanged.
- `TestExperimentInterlockStopsEffects`: the fixed `time.Sleep(time.Second)` (T5) is gone. The test now waits for the durable fact that stops the effect, a `policy_evaluations` row with `result = 'denied'` and a reason starting `interlock_not_ready` (the policy plane denies before any command exists), then asserts no non-denied evaluation, no `commands` row and no command at the device. It is a stronger proof than a one-second wait.
- `waitForRow` and `waitReady` poll with a ticker bound to `t.Context()` and a timeout (`pollUntil`, 25 ms) instead of `time.Sleep`.
- The experiment `serve` database and every operator-command, `run-live`, `serve` and explain test database is seeded from the migrated template (`seedMigratedDatabase`, `newMigratedDatabasePath` over `storagetest.Open`), so the commands under test open a migrated file as in production after the first start. The first-start migration is proven in `internal/storage`.
- The recorded-replay checks (live run verifies, tampered decision refused, never-deployed spec refused) are three parallel subtests; the tampered/verify replays overlap instead of running one after another.
- Every replay a test starts (`--repeat`, recorded, shadow, plain `run`) opens its isolated database from the migrated template: the test passes `seededReplayContext(t)` (`replaytest.WithDatabaseOpener(t.Context(), storagetest.Open)`, round 14) to `ExecuteContext`, which the commands hand to `replay`. The production `run` command is unchanged and keeps `storage.OpenFresh`; the real opener stays proven in `internal/replay` and `internal/storage`. Package 15.1 s → 9.6-11.1 s; `TestRunRepeatProvesDeterminism` 2.1 s → 0.2 s alone; each recorded-replay subtest 3.5 s → 1.5-2.1 s; shadow 2.8 s → 1.6 s.
- Process environment (OTLP exporters off, the subscriber token) is set once in `TestMain` instead of per test with `t.Setenv`, which made 8 tests non-parallel; the experiment tests keep their single process-wide environment, now in one place.
- One shared read-only fixture, `watchRunDatabase` (`sync.OnceValues`, removed by `TestMain`), replaces a per-test run-live of the watch trace.
- Tried and rejected: `--poll-interval` 10 ms (slower, 7.3 s ingest), 250 ms (faster ingest, slower dispatch, net equal), live feed reading interval 500 ms (no change).

### Removed
- The `unknown trace format` row of `TestRunLiveRejectsInvalidInputsBeforeOpeningState`: T2 (`TestRunTraceRejectsUnknownFormat` owns it) and a false claim. The CLI only validates the format after it has opened the database, which cost a cold migration (1.7 s) in a test named "before opening state". The remaining rows now assert that the database file was never created.
- `disableTelemetryExport`, `experimentEnvironment`, `useExperimentEnvironment` and the `countLogged` helper: replaced by `TestMain` and `countRows`.

### Renamed or moved
- `TestRunRepeatProvesDeterminism` moved from `main_test.go` to `run_command_test.go` (T3: file names the subject; `main_test.go` keeps the version and serve-validation tests). Its refusal of `--repeat` with `--db` now asserts the message ("its own fresh databases (no --db)").
- `poll_test.go` content → `helpers_test.go` (`pollUntil`, `seedMigratedDatabase`, `newMigratedDatabasePath`, `countRows`).

### Improved
- T6: 23 hygiene findings fixed; all tests are parallel except those that use `t.Setenv` for the behavior under test (`TestServeFlagsValidate` sets the subscriber token per case, `TestServeApprovalConfiguration` and `TestServeMountsApprovalsOnlyForConfiguredPipeline` set the approval token and relay). They run first and sequentially and take milliseconds.
- T6: `claimUnclaimedAddress` took a package-level `sync.Map`; with `-count>1` the two tests of the claim helper failed ("no unclaimed loopback port after 50 picks") because the map outlived the first iteration. Each test now passes its own map. Found by `-count=3`.
- T4: `TestRunLiveRejects...` asserts no state was created; the experiment inspection asserts the human text of `episode show` and `intent show`.
- `thelper`: `streamReadings`, `thermalReadings`, `thermalReading` call `t.Helper()`.

### Added
- `TestValidatePrintsTheDigestsTheBenchGatewayAllowLists`, `TestValidateJSONPrintsTheCanonicalSpec`, `TestValidateRefusesASpecThatDoesNotCompile`: the `validate` command (0% covered) is what the runbook uses to print the policy digest the bench gateway allow-lists; the output is compared with the compiler's and `policy.DigestForVersion`'s values.
- `TestRunReplaysTheTraceIntoItsDatabase`: plain `run` (0% covered) prints non-zero event and version counts and a 64-digit hash and leaves Situation versions in `--db`.
- `TestSituationListAndShowPrintReadableText`, `TestExplainPrintsDerivationEvidenceAndTriggersAsText`: the human output of `situation list|show`, `explain situation|trigger` (0% covered), including the empty list for an unknown entity.
- `assertInspectionText` in the closed-loop experiment test: `episode show` and `intent show` text lead from the episode to the policy evaluation, command, outcome and verification.
- Coverage 74.3% → 83.1%; still uncovered: the `commands list` row format and `commands resolve` success (need a command in an uncertain state), approval composition (needs a pipeline), shadow text output (one more cold replay), physical-gateway wiring.

## Production code touched
- none.

## Invariants proven here
- Experiment compatibility (X01) and invariants 2, 6, 8: `TestExperimentClosedLoopThroughServe`, `TestExperimentClosedLoopUnderAContinuousFeed`, `TestExperimentInterlockStopsEffects`, `TestExperimentShadowReplayComparesTheCandidate`, `TestExperimentCommandsKeepTheirFlags`.

## Open items
- Resolved: no test in this package pays a cold `storage.OpenFresh` migration any more (see Speed).
- `run-live` validates `--trace-format` only after opening the database, so a bad format costs a migration; validating it in `validateLiveBatch` would be a behavior change (error precedence), not done.
