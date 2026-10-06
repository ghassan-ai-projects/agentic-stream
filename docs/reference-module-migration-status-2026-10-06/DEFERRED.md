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
- **layer table**: engine 10, runtime/internal/app 11, composition 12, runtime 13, replay store/app/facade 11/12/13, cmd 14.

## Deferred work

1. **Owner-provided read ports** for the authority projection. Actions, policy and cognition join foreign tables (`intents`, `decisions`, `episodes`, `situations`, `approvals`, `policy_evaluations`). It needs one cross-module design touching five owners' public APIs; not started.
2. Typed provider and observed-effect results (actions); typed simulator records (ingress); typed operator-state and timer-payload records shared with `operators` (engine).
3. One transaction spanning an ingress batch and its checkpoint; one transaction per global engine batch; per-watch fan-out in one transaction.
4. Re-level the layer table from the import graph before layering `qualification` or the ledgers. `notify` and `control` were layered by moving `approvalledger`, `policy` and `runartifact` up one level each (see its record).
5. Review the shape of `executor/native`, `executor/remote`, `runartifact`, `worker`.

## Open decisions (see DEADCODE.md)

Wire or remove: interlock trip path, replay shadow/counterfactual/baseline modes,
calibration activation, notification pruning (kept as `notify.Service.Prune`, unscheduled until a retention is chosen), quarantine release/redrive.

## Validation at this commit

`make ci-check` passes (proto-check not run locally: needs the pinned protoc);
`go test -race -count=1 ./...` passes.
