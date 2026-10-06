# Behavior changes, deferred work and open decisions

## Deliberate behavior changes made in this session

Each is also recorded in the module's `PLAN.md`.

- **actions**: `New(Config)` requires database, effector, runtime-ownership check and interlock; `Config.Effector` is an `AuthorizedEffector`, refused at construction. Reconciling a `manual_review` command now settles its status and verification. Command, intent, decision and evidence documents parse into typed values (raw maps remain digest input).
- **watch**: owner and interlock are constructor requirements; `Fire` became private; the type is `Service`.
- **engine**: `New(ctx, Config)` replaces `NewEngine`/`NewStreamEngine`/`WithRuntimeOwner`; ownership is required and replay passes `engine.ReplayOwnership`; `Run`/`RunDueTimers` are test-only.
- **ingress**: `New(Config)` with `ReplayJSONL`, `ReplaySimulator`, `ServeLive`; a storage error reading a simulator checkpoint stops the replay instead of restarting at line 0; both checkpoint writers share one upsert and codec; simulator data moved to `internal/ingress/internal/domain/simulator_data.json`.
- **runtime composition**: composition without a runtime owner passes an explicit always-pass check; a non-authorized effector fails composition.
- **notify**: `New(db)` replaces the free `ReadPage`/`Prune` functions and rejects a nil database; `AppendLifecycleEventWithTrace` became `AppendLifecycleEvent(LifecycleEvent)` with typed payloads (the payload fixes the event type; the domain stamps tenant and source authority); `notifycontract` merged into `notify`; the SSE handler answers 503 `runtime_not_ready` when the service cannot be built; audit details marshalling errors are returned.
- **control**: `costcontrol` merged into `control` (`CostLedger`, `SetCostLimit`, `ApplyCostCeilings`, `ErrCostReservationRejected`); `episodeledger` recovery takes a `CostSettler` port; control is layered facade/app/domain/store, which moved `api`, `authority`, `soak`, `actions`, `device`, `episodes` and the executors up a level.
- **episodeledger**: `scheduleledger` merged into `episodeledger` (queue operations renamed `...SchedulerItem...`); `RecoverUnfinishedAttemptsWithCost` is `RecoverUnfinishedAttempts` with an optional `CostSettler`; layered facade/app/domain/store, which moved the executors, `episodes`, `actions`, `device`, `control`, `admission` and `runtime` up one level; a few error texts changed (see its PLAN).
- **approvalledger**: layered facade/app/domain/store; `WithdrawSuperseded` takes a `WithdrawalPublisher` instead of `(tenantID, clock)` and the ledger no longer imports `notify` (the cognition store builds the `approval.withdrawn` event); a nil publisher is refused; superseded approvals are listed before any write; the ledger sits at layer 3 instead of 5.
- **qualification**: dissolved. `CalibrationStore.Activate` and its helpers were deleted (no production caller); `ShadowDecision`/`ShadowScore` and the insert moved into `episodes` (the `ShadowStore` config field and its nil checks are gone); `ShadowComparison` was merged into `replay`'s existing `Comparison`; the calibration check became `policy`'s store query `CalibrationActive`, and `policy.Config.Calibration`/`CalibrationCheck` and the runtime's `policyCalibrationCheck` were removed.
- **admission**: merged into `runtime` (`app.NewAdmitter`, `store.PipelineStore.InAdmission`, `domain.RefuseFixture`); the admitter requires its store, episode assembler and clock at construction; the runtime store sits at layer 10 because it now assembles episodes.
- **ids**: `Sequence`/`NewSequence` (test-only) and the unused `PrefixArtifact`/`PrefixReplay` were deleted; `contractsv1`'s unused duplicate prefix registry and `ID` type were deleted (it keeps `TenantID` and `PartitionCount`); `situations`, `cognition` and the fixture executor use the `ids.Prefix*` constants instead of literals; notification audit ids use `ids.PrefixAudit` (was `pol_`).
- **canonicaljson**: kept as the layer-0 foundation. `MarshalString` (test-only) and `DomainTest` (used only by its own tests) left production code; fourteen files that hand-wrote `"sha256:" + hex.EncodeToString(...)` now call `canonicaljson.EncodeDigest`, and the remote executor's `fmt.Sprintf("sha256:%x")` does too.
- **layer table**: engine 10, runtime/internal/app 11, composition 12, runtime 13, replay store/app/facade 11/12/13, cmd 14.

## Deferred work

1. **Owner-provided read ports** for the authority projection. Actions, policy and cognition join foreign tables (`intents`, `decisions`, `episodes`, `situations`, `approvals`, `policy_evaluations`). It needs one cross-module design touching five owners' public APIs; not started.
2. Typed provider and observed-effect results (actions); typed simulator records (ingress); typed operator-state and timer-payload records shared with `operators` (engine).
3. One transaction spanning an ingress batch and its checkpoint; one transaction per global engine batch; per-watch fan-out in one transaction.
4. Re-level the layer table from the import graph (the ledgers and `qualification` are done: see their records). `notify` and `control` were layered by moving `approvalledger`, `policy` and `runartifact` up one level each (see its record).
5. Review the shape of `executor/native`, `executor/remote`, `runartifact`, `worker`.

## Open decisions (see DEADCODE.md)

Wire or remove: interlock trip path, replay shadow/counterfactual/baseline modes,
calibration activation, notification pruning (kept as `notify.Service.Prune`, unscheduled until a retention is chosen), quarantine release/redrive.

## Validation at this commit

`make ci-check` passes (proto-check not run locally: needs the pinned protoc);
`go test -race -count=1 ./...` passes.
