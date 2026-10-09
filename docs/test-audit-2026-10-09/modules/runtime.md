# runtime

Status: done
Round: 15

Audited in round 15 with [api](api.md). runtime owns the live pipeline,
readiness and worker facades. Two production changes were needed, both
behavior-preserving (see [Production code touched](#production-code-touched)).

## Metrics

Times are `go test -short -race -count=1` wall time per package, measured while
other workers were running (load average 5 to 8). Tests are top-level tests /
passing subtests.

| Package | Coverage before | after | Time before | after | Tests before | after |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/runtime` (facade) | 83.3% | 100.0% | 2.0 s | 1.9 s | 5 / 0 | 8 / 2 |
| `internal/runtime/internal/app` | 80.3% | 89.8% | 6.0 s | 3.1 s (3.7 s at load 8) | 37 / 8 | 38 / 43 |
| `internal/runtime/internal/composition` | 78.6% | 79.6% | 2.1 s | 2.0 s | 6 / 0 | 7 / 2 |
| `internal/runtime/internal/domain` | 100.0% | 100.0% | 1.2 s | 1.1 s | 5 / 11 | 7 / 26 |
| `internal/runtime/internal/store` | 68.2% | 83.5% | 1.8 s | 2.1 s | 6 / 3 | 16 / 7 |
| `internal/runtime/internal/transport` | 88.6% | 92.1% | 1.7 s | 1.7 s | 7 / 3 | 17 / 5 |

Hygiene lint (paralleltest, tparallel, usetesting, thelper): 39 findings before
(app 22, transport 7, store 4, composition 2, domain 2, facade 2), 0 after. Repository
`golangci-lint`: 0 issues. `dupl` at threshold 60: 0. `time.Sleep` in tests: 4
before (`pipeline_clock_test.go`, `live_socket_test.go`), 0 after.
`go test -race -shuffle=on -count=3`: passes.

Mutation checks (each reverted): the drain refusal in the admitter, the owner
epoch stamp on an admitted episode, device routes falling through to the
fallback, the owner check before a live socket source runs, the heartbeat
clearing readiness, the ledger/owner epoch match in recovery; every one fails
named tests.

## Proving tests

| Behavior | Test |
| --- | --- |
| Readiness is gated on the claim, recovery and heartbeat; it is final | `TestServiceIsReadyOnlyBetweenStartAndClose`, `TestTheHeartbeatClearsReadinessWhenTheOwnerLeaseCannotBeRenewed`, `TestAFailedRecoveryLeavesTheServiceNotReady` (app); `TestRecoveryCoordinatorAtomicallyRecoversEpisodesAndEvidence`, `TestAFailedClaimRecoversNothing`, `TestRecoveryWithoutAClockOverrideUsesTheClaimTimestamp` (store); `TestServiceFacadeIsReadyOnlyBetweenStartAndClose` (facade) |
| A killed or drained epoch stops new work but not ingestion | `TestADrainedOrKilledEpochKeepsIngestingButStartsNoNewWork`, `TestAdmitPendingAdmitsOrSkipsTheDueItemAccordingToTheScenario` (drain case) |
| A pipeline that lost ownership ingests nothing | `TestAPipelineThatLostOwnershipIngestsNothing`, `TestEveryOwnerAssertionGoesThroughTheOneRuntimeOwnerCheck` (store) |
| Source ordering and no head-of-line blocking | `TestPipelineKeepsIngestingWhenCostReservationIsRejected`, `TestEpisodesRunBesideIngestion`, `TestExpiredItemsNoLongerFillGlobalCapacityOrTheNextPoll`, `TestALiveSocketEventFlowsThroughEveryGovernedStageUntilShutdown` |
| Live source never replaces an existing file | `TestALiveSocketNeverReplacesAnExistingFile` |
| Dispatch policy: fixture refused on production, shadow never dispatches | `TestDispatchPolicyDecidesWhetherAnExecutorIsAdmittedAndWhetherItsDecisionsReachActions` |
| Effect routing fails closed; authorization is kept | `TestDeviceRoutesNeverReachTheFallback`, `TestARouteWithoutItsEffectorFailsClosedInsteadOfFallingBack`, `TestAuthorizedDispatchKeepsTheFinalAuthorizationCheck` |
| Worker connection and teardown order | `TestWorkerSetupOrderAndCleanupAfterAFailedStep` (app); `TestBackendServesEvidenceThenConnectsTheWorkerAndReleasesBoth`, `TestConnectWorkerReachesTheWorkerListeningOnItsSocket` (transport) |
| Maintenance without sensor input | `TestStartMaintainsWatchesWithoutNewSensorInput`; the approval proof is policy's `TestACommittedApprovalDispatchesWithoutNewSensorInput` |

## Findings and changes

### Removed
- `TestPipelineCorrectsLateWindowAndAdmitsOneReconsideration` (app
  `pipeline_e2e_test.go`): it ran the first batch through the pipeline and the
  late batch by hand through ingress and the engine, then asserted what the
  cognition and engine tests own. Its distinct assertions (one corrected version,
  one reconsideration) moved into the reconsideration test below (T2).
- `TestEpochKillRefusesLaterDecisions`: it killed an epoch and asserted `control`'s
  own `AssertDecision`, including that another epoch is untouched; that is
  `control`'s contract, covered by its tests (T2).
- `watch_fixture_test.go` and `watch_fixture_internal_test.go`: identical files
  in two packages; one `NewWatch` in `export_test.go`.
- Three copies of the same ten-field `spec.CompiledSpec` literal, two copies of
  the trace writer and two of `runtimeTicketSchema`: one `thingSpec` with
  options, one `traceEvent` builder (T9).

### Renamed or moved
- Readiness: `TestServiceReadinessFollowsRecoveryAndClose` existed in the facade
  and in `app`; each is renamed to say the behavior
  (`TestServiceIsReadyOnlyBetweenStartAndClose`,
  `TestServiceFacadeIsReadyOnlyBetweenStartAndClose`); the facade keeps its copy
  because it proves the delegation, not the rules.
- app: `costcontrol_test.go` → `cost_rejection_test.go`;
  `dispatch_mode_test.go` → `dispatch_policy_test.go`;
  `pipeline_e2e_test.go` → `pipeline_end_to_end_test.go`;
  `pipeline_clock_test.go` → `scheduled_advance_test.go`;
  `pipeline_episodes_test.go` → `episodes_beside_ingestion_test.go`;
  `pipeline_live_socket_test.go` (it proved watch firing, not the live socket) →
  `watch_firing_test.go`; `runtime_mode_fixture_test.go` → `fixtures_test.go`;
  `worker_test.go` → `worker_setup_test.go`;
  `effect_routing_integration_test.go` merged into `effect_routing_test.go`;
  `watch_fixture_internal_test.go` → `export_test.go`.
- The "p8" names (`p8-drain.db`, `evt-p8`, `TestDispatchMode...`) are gone (T3).
- facade: `reconsideration_test.go` (in `package runtime`, reaching into
  `internal/app` and `internal/store`) → app `reconsideration_test.go`.
- store: `recovery_clock_test.go` merged into `recovery_test.go`; the recovery
  fixture is `recovery_fixture_test.go`; `pipeline_test.go` is split by subject
  into `pipeline_test.go`, `owner_test.go`, `costs_test.go`, `approvals_test.go`,
  `admission_test.go`.
- transport: `worker_test.go` → `worker_backend_test.go`.
- Documentation links updated: `documentation/learn/how-it-works.md`,
  `documentation/learn/time-and-state.md`, `documentation/guides/predictive-maintenance.md`,
  `documentation/design/replay-and-shadow.md` (dated archives under `docs/` keep
  their historical file names).

### Improved
- T5: `time.Sleep(100ms)` (check that nothing was admitted early) is replaced by
  the report of the batch that ran at ingestion time (zero admitted while the
  debounce runs) followed by a virtual-clock jump; the 10 ms polling loops are a
  `waitUntil` helper on a 1 ms ticker bound to a 10 s timeout; the live socket
  test dials with that helper instead of `os.Stat` + sleep.
- T6: every test and subtest is parallel except `TestNativeBackendUsesTheConfiguredModelEndpointWithTheKeyFromTheEnvironment`
  (`t.Setenv`; the deterministic-provider case moved to its own parallel test).
- Unix sockets: `workerfake.SocketDir` replaces `/tmp/agentic-stream-runtime-live-<nanos>.sock`
  and hand-rolled `os.MkdirTemp`.
- `context.Background()` replaced by `t.Context()` in every test.
- T4: `t.Fatal(found, err)` style messages state the expectation; the shadow
  check asserts all three counts with got and want.
- Tables: the dispatch-mode matrix (7 cells, was 7 tests with a shared helper),
  admission scenarios (6 cases, each also proving a second poll admits nothing),
  epoch drain/kill, effect routing, worker setup, store recovery configuration.

### Added
- app: `TestAPipelineThatLostOwnershipIngestsNothing` (all three sources);
  `TestALiveSocketNeverReplacesAnExistingFile`;
  `TestPipelineIngestsASimulatorTraceThroughTheSameGovernedStages` (0% before);
  `TestPipelineNamesTheSourceThatFailedToIngest`;
  `TestStartMaintainsWatchesWithoutNewSensorInput` (maintenance loop, 0% before)
  and `TestMaintenanceStartsOnceAndCanBeRestartedAfterClose`;
  `TestWatchMaintenanceFailureStopsLaterBatches`;
  `TestWatchFiringOnlyReadsEventsAfterTheBatchStartPosition`;
  `TestScheduledLoopsRefuseANonPositiveInterval` and
  `TestScheduledLoopsStopWithTheFailureThatBrokeThem`;
  `TestTheHeartbeatClearsReadinessWhenTheOwnerLeaseCannotBeRenewed` (the old test
  called the renewal and `setNotReady` by hand and never ran the heartbeat);
  `TestAFailedRecoveryLeavesTheServiceNotReady`;
  effect routing: watch route before fallback, unconfigured routes fail closed,
  authorization denial and missing authorization, an effector that cannot enforce
  authorization, device verification (5 cases).
- store (68.2% to 83.5%): scheduler queue per tenant and its failure, an
  admission transaction commits together or not at all, cost-rejection and expiry
  record why an item left the queue, approvals run in one fenced transaction
  and report a transaction that cannot begin, recovery refuses an incomplete or
  mismatched configuration and a failed claim recovers nothing, empty cost
  ceilings touch nothing.
- transport (88.6% to 92.1%): a real worker listening on a Unix socket answers a
  handshake through `ConnectWorker`; the connect step that refused is named;
  `WorkerOptions` projects every field; sources refuse missing dependencies.
- composition: the pipeline records configured cost ceilings, defaults fill only
  what is unset, the public `NewWorkerRuntime` validates before opening resources.
- facade (83.3% to 100%): every exported `Pipeline` operation delegates
  (`RunJSONL`, `RunSimulatorJSONL`, `RunLiveSocket`, `AdvanceEvery`,
  `RunEpisodesEvery`, `ApprovalForSigning`, `ResolveApproval`), construction
  refusals, worker route selection.
- domain: `MaintenanceInterval`, `DecodeEvidenceKey`, route classification edge.

### Speed
- `internal/runtime/internal/app`: 6.0 s to 3.1 s. The package was sequential
  (nothing parallel, about 90 database tests at 0.1 s each under `-race`). All
  tests are parallel now.
- `TestFireRecentWatchesPaginatesPastFullPage` (1.5 s, the slowest test) appended
  and evaluated 1001 events to cross the 1000-row page boundary. The page size is
  a `Pipeline` field now (default unchanged), the test uses a page of 3 and 7
  events (see Production code touched).
- `TestEpisodesRunBesideIngestion` dropped to a 10 ms schedule after an
  attempt with 1 ms made the loops starve ingestion under `-race`.
- policy's `TestACommittedApprovalDispatchesWithoutNewSensorInput`: see below.

## Production code touched
- `internal/runtime/internal/composition/pipeline.go`, `planes.go`;
  `internal/runtime/internal/app/pipeline_construction.go`, `pipeline.go`;
  `internal/runtime/internal/domain/lifecycle.go`: new field
  `PipelineConfig.MaintenanceInterval`. `Pipeline.Start` maintained watches and
  approved-command delivery on a fixed `time.NewTicker(time.Second)`; it now uses
  `domain.MaintenanceInterval(configured)`, which returns one second when the
  field is unset or not positive. No caller sets it in production, so the
  default and the behavior are unchanged. Round 11's
  `TestACommittedApprovalDispatchesWithoutNewSensorInput`
  (`internal/policy/internal/app/approval_dispatch_test.go`) now sets 10 ms.
  Before and after, three runs each under `-race -short`: test 1.24 s, 1.24 s,
  1.24 s → 0.26 s, 0.25 s, 0.26 s; package `internal/policy/internal/app`
  2.6 to 2.75 s → 1.65 to 1.77 s. `internal/runtime/README.md` states the
  option.
- `internal/runtime/internal/app/pipeline.go`, `pipeline_run.go`:
  `watchReadBatchSize` (const 1000) became `defaultWatchPageSize`, and the page
  read in `fireRecentWatches`/`fireWatchPage` goes through `Pipeline.watchPage()`,
  which returns a `watchPageSize` field when set and 1000 otherwise. The field is
  only set by the pagination test; production never sets it.

## Invariants proven here
- Invariant 4 (deterministic state changes are serial per partition): batches
  and live events hold one batch lock; `TestEpisodesRunBesideIngestion` proves
  episodes run beside, not inside, the serial ingestion.
- Invariant 6 (a model cannot execute effects): `TestDeviceRoutesNeverReachTheFallback`,
  `TestARouteWithoutItsEffectorFailsClosedInsteadOfFallingBack`,
  `TestAuthorizedDispatchKeepsTheFinalAuthorizationCheck`; shadow decisions create
  no intents or commands (`assertShadowWithoutEffects`).
- Invariant 7 (policy revalidates before dispatch): the pipeline evaluates every
  pending intent before dispatch (`TestPipelineCompletesDecisionToSimulatedOutcome`
  counts one evaluation per command).
- Invariant 8 (stable identities, inbox/outbox): the recovery tests prove that an
  interrupted attempt and evidence call are repaired atomically with the owner
  claim; no duplicate effects come from `TestStartMaintainsWatchesWithoutNewSensorInput`
  because dispatch is outbox-driven.
- Invariant 10 (explainability): skipped, quarantined, cost-rejected and expired
  scheduler items keep a reason in `trigger_evaluations`
  (`TestAdmitPendingAdmitsOrSkipsTheDueItemAccordingToTheScenario`,
  `TestAdmissionStepsRecordWhyAnItemLeftTheQueue`).

## Open items
- Production finding (not changed): cancelling the context while a live-socket
  batch is still committing surfaces a non-cancellation error from
  `RunLiveSocket`. `TestALiveSocketEventFlowsThroughEveryGovernedStageUntilShutdown`
  failed 6 times in 180 runs (12 copies of the test binary at once, `-race
  -shuffle=on`) with `run live socket source: process live ingress line: dispatch
  action: lease command: commit tx: sql: transaction has already been committed
  or rolled back`. After the command row reached `succeeded` the batch was still
  in its dispatch loop (`dispatchApprovedCommands` leases once more to find the
  queue empty); the test's cancel rolled that lease transaction back.
  `database/sql` reports `ErrTxDone`, which is not `context.Canceled`, so
  `domain.NormalLiveSocketShutdown` (it accepts only cancellation or deadline
  errors) treats a clean shutdown as a failure. `AdvanceEvery` and
  `RunEpisodesEvery` already ignore errors once the context is cancelled; the
  live socket does not. Fix options for the owner: classify a transaction ended
  by a cancelled parent context as a normal shutdown, or let the sink check
  `ctx.Err()` before returning. The test now runs a `RunJSONL` on an empty trace
  after the asserted row: it takes the same batch lock, so it returns only when
  the live event's batch has finished, and only then cancels. 0 failures in 300
  runs under the same load. The other new shutdown paths were audited:
  `AdvanceEvery`/`RunEpisodesEvery` tests (errors ignored once cancelled),
  `Pipeline.Close` after the maintenance test (returns nil; cleanup runs before the
  database closes), the facade test (closed in cleanup) and the ownership-loss live
  source (bounded context, expects the ownership error), none have the pattern.
- `TestWalkthrough` is a documented manual tool
  (`docs/walkthrough-end-to-end-2026-10-06/README.md`) that skips unless
  `AGENTIC_STREAM_WALKTHROUGH` names an output database. It is the one skip that
  is neither `-short` nor a missing optional tool; moving it to a `go run`
  program under `docs/` is a documentation decision, not a test one.
- Production slog warnings and errors from skipped or quarantined items go to
  stderr and flood `go test` output (a hundred lines in the capacity test); a
  logger injected through the pipeline config would silence them.
- `composition.composeEffectors` has no test with a `GatewayEffector` (the device
  module owns its behavior; constructing one needs a gateway link).
- `AdmissionTx.Assemble` and `Persist` are covered only through the app admission
  tests, not by the store package alone: they need an assembled episode request.
- `Pipeline.AdvanceEvery` takes the batch lock for a full batch every tick; at a
  1 ms interval it starved ingestion under `-race`. Not a bug at production
  intervals (the CLI uses seconds), but an interval floor may be worth a decision.
