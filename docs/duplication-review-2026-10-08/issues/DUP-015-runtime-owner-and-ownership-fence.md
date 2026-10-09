# DUP-015: The runtime_owner lease check is re-implemented in episodeledger and the ownership fence port is declared about ten times

- Status: fixed
- Severity: medium
- Verdict (finders): DIVERGED
- Themes: mechanisms
- Wave: 3
- Finder sources: M2, M3 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Inject `control`'s owner assertion into `episodeledger` instead of re-querying `runtime_owner` (a direct import would cycle). Declare the fence function type once (alias in `storage`) and make the 'no owner bound' policy a single decision. Keep each module's error text where tests match it.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report M2: `runtime_owner` lease check re-implemented in episodeledger, with different semantics

- Verdict: DIVERGED
- Shared meaning: "this epoch currently owns an unexpired runtime lease".
- Sites:
  - internal/control/internal/store/owner.go:96-103 `HoldsLease`: `owner_epoch = ? AND owner_instance = ? AND lease_until > ?`, time via `domain.TimeText` (fixed-width). Called from control/internal/app/owner.go:99 (`assert`).
  - internal/episodeledger/internal/store/fence.go:92-100 `OwnerHoldsLease`: `SELECT owner_epoch FROM runtime_owner WHERE singleton_id = 1 AND owner_epoch = ? AND lease_until > ?`, no `owner_instance`, time via `store.TimeText` (variable width). Called from episodeledger/internal/app/identity.go:52 `requireOwnerLease`.
  - Same foreign read of another table by episodes: internal/episodes/internal/store/dispatch.go:57 (`epoch_control ... state = 'killed'`) and control/internal/store/epoch.go:76 (`state <> 'killed'`); the Go constant is control/internal/domain/epoch.go:22.
- How they differ: instance ignored in episodeledger (a second process with the same epoch string but another instance passes the episode fence but fails control's); different time encoding (C1); error mapping differs (control returns `ErrRuntimeOwnerBusy`, episodeledger maps to `RejectStaleAttempt`); 'killed' is a SQL literal in two stores outside the module that owns the state names. (known: #5, #3b)
- Risk if left: changing lease semantics in control (e.g. grace period, instance rule) does not reach attempt fencing; the table's owner can change columns and break a foreign SELECT silently.
- Proposed canonical owner: control (owns `runtime_owner` and `epoch_control`). episodeledger cannot import control today (allowedImports for episodeledger/internal/app = domain+store only, and control/internal/store imports episodeledger, so control -> episodeledger already exists; the reverse would be a cycle).
- Proposed fix: inject the check the way every other module does: `episodeledger` takes `OwnerLease func(ctx, *sql.Tx, epoch string) error` (the `control.RuntimeOwner.Assert` method value, same as engine/watch/actions), invoked in `requireOwnerLease` instead of `tx.OwnerHoldsLease`; delete `OwnerHoldsLease`. For episodes dispatch, expose from control a read (`EpochControl.KilledEpochs` or a boolean per epoch) or pass the predicate result in; delete the literal 'killed' from episodes' SQL.
- Behaviour to preserve: `RejectStaleAttempt` for a lost lease (domain.Refuse); identity without owner epoch is not fenced (identity.go:47); dispatch of killed-superseded unstarted episodes (dispatch.go:50-58 comment).
- Verification: episodeledger fence/stale-attempt tests; control recovery_fence_test.go. New test: attempt transition with lease held by the same epoch but another instance must be refused (pins the semantic that is currently only incidental).

### Finder report M3: Ownership fence port declared and applied in ~10 places; "no owner bound" encoded 4 ways

- Verdict: DIVERGED
- Shared meaning: a module runs `control.RuntimeOwner.Assert(ctx, tx, epoch)` (or the epoch variants) on its own transaction before a write; failing means ownership lost.
- Sites (the func type `func(context.Context, *sql.Tx, string) error`, each named differently): evidence/internal/store/tx.go:13 `OwnerCheck`; actions/internal/store/tx.go:15 `OwnerCheck`; watch/internal/store/tx.go:13 `OwnerCheck`; engine/internal/store/tx.go:17 `OwnerCheck`; episodes/internal/store/tx.go:34 `DecisionEpochCheck`; policy/internal/store/tx.go:12 `Fence`; authority/internal/store/store.go:41 `Fence`; interlock/interlock.go:35 `Fence` (2-arg form); policy/internal/app/principals.go:16 `Ownership{Check, Epoch}`. Config fields declaring the raw func again: evidence/config.go:21, episodes/config.go:32, actions/service.go:29 and reconciler.go:20, watch/service.go:22, engine/config.go:30, policy/config.go:17.
- Sites (Tx.AssertOwner wrappers, each wraps with its own text): evidence tx.go:51 "evidence ledger runtime ownership lost"; actions tx.go:59-68 "action runtime ownership lost" (plus an explicit nil check that Store.Configured() already guarantees); watch tx.go:48-54 "assert watch runtime owner"; engine tx.go:96-101 "stream runtime ownership lost"; policy/internal/app/audit.go:12-17 "policy runtime ownership lost"; authority/internal/app/tx.go:30-37 "runtime authority:" + "epoch control:"; runtime/internal/store/pipeline.go:28-37 and admission.go:35-43 "runtime ownership lost" (two near-identical copies); runtime/internal/store/costs.go:41-48 "assert owner for cost configuration".
- Sites ("no owner bound = unfenced", four encodings): runtime/internal/composition/planes.go:124-129 `runtimeOwnershipCheck` + :139 `unownedCheck` (a fake check returning nil); runtime/internal/store/pipeline.go:29 (`Owner == nil || OwnerEpoch == ""` returns nil); admission.go:36 (same test, same result); costs.go:42 (inverted form of the same test).
- How they differ: error text differs per module (no caller matches the text; all keep `%w` so `errors.Is(ErrRuntimeOwnerBusy)` still works, device/internal/app/session_reconcile.go:93); pipeline.go:AssertOwner opens a separate transaction (checks ownership outside the work's tx) whereas the other nine run on the work's tx; only actions guards nil owner inside Tx, the rest rely on Configured().
- Risk if left: a new plane copies one of ten shapes; the unowned mode has to be changed in four predicates; a rename of the fence signature touches 15 declarations.
- Proposed canonical owner: `internal/storage` (leaf; every store already imports it): `type TxCheck = func(context.Context, *sql.Tx, string) error` plus `func (TxCheck) Wrap(what) ...` is not needed; one alias is enough. Keep control as the implementer. No new import edges: all listed stores already import storage; facade packages evidence/episodes/actions/watch/engine/policy import storage or can use the alias via their store (evidence, engine, watch, actions facades already list internal/storage).
- Proposed fix: add `storage.OwnerCheck`; delete the 8 local named types and use the alias in Config and Store. In composition, build `runtimeOwnershipCheck` once (already) and make runtime store receive that closure instead of (`Owner`, `OwnerEpoch`) pairs: `PipelineStore.AssertOwner`, `AdmissionTx.AssertOwner`, `assertCostConfigurationOwner` then call one `fence(ctx, tx)` and the nil test lives in `runtimeOwnershipCheck` only.
- Behaviour to preserve: wrapped error chain (`errors.Is` with ErrRuntimeOwnerBusy/ErrEpochKilled in device and policy), per-module message prefixes only if a test asserts them (tests use `errors.New("ownership lost")` stand-ins: actions/internal/app/service_test.go:52, watch service_test.go:48, watch store_test.go:131); unowned mode (cfg.Owner == nil) still runs without fencing for replay/tests; pipeline.AssertOwner semantics (outside tx) if intentionally a preflight.
- Verification: the three tests above; composition pipeline_test.go. New test: runtime store with no owner configured asserts nothing and with owner lost refuses, through the single closure.

## Outcome

Status: fixed. Commit: 60c7b98.

### Verified

- M2 sites confirmed: `episodeledger` re-queried `runtime_owner` (`OwnerHoldsLease`) with `owner_epoch = ? AND lease_until > ?` and no `owner_instance`, while `control` `HoldsLease` also compares `owner_instance`. Confirmed real divergence: a second process holding the same epoch string under another instance passed the attempt fence and failed control's.
- M2 time-encoding claim is partly stale: at HEAD both sides already encode with `kernel.FormatTime` (fixed width), so the C1 encoding divergence no longer existed. The clock still differed: the ledger judged the lease at `time.Now()` (hard-wired in the facade `TransitionAttempt`), control at its injectable `Owner.Now`.
- M2 'killed' literal: control's `epochControlUpsert` repeated the literal that `domain.EpochKilled` owns (confirmed). The foreign read moved: it now lives in `episodeledger/internal/store/dispatch.go` (`killedUnstartedEpisodePredicate`), not `episodes`.
- M3 confirmed: ten func types of the shape `func(context.Context, *sql.Tx, string) error` (evidence/actions/watch/engine `OwnerCheck`, episodes `DecisionEpochCheck`, policy and authority `Fence`) plus raw-func Config fields in evidence, episodes, actions (two), watch, engine, policy; and four encodings of "no owner bound" (composition `unownedCheck`, runtime store pipeline/admission/costs predicates). `interlock.Fence` is a 2-argument form of a different check and is not part of this type; left alone.
- M3 claim "no new import edges" is wrong for application layers: `TestApplicationLayersDoNotTouchInfrastructure` forbids `internal/<m>/internal/app` from importing `internal/storage`. Apps that name the type (policy, authority, episodes, episodeledger) get it from their store (see Decisions).
- M3 claim "pipeline.AssertOwner opens a separate transaction" confirmed and kept (it is a preflight outside the work's transaction); the other two predicates ran on the work's transaction and still do.

### Changed

Fence type declared once:
- `internal/storage/internal/store/owner_check.go` (new): `type OwnerCheck = func(context.Context, *sql.Tx, string) error`; `internal/storage/storage.go` re-exports it as `storage.OwnerCheck`.
- Deleted the local named func types and used the alias: `evidence`, `actions`, `watch`, `engine` `internal/store/tx.go` (`storage.OwnerCheck`); Config fields in `evidence/config.go`, `episodes/config.go`, `actions/service.go`, `actions/reconciler.go`, `watch/service.go`, `engine/config.go`, `policy/config.go`.
- `episodes/internal/store/tx.go` (`DecisionEpochCheck`), `policy/internal/store/tx.go` (`Fence`), `authority/internal/store/store.go` (`Fence`) and `episodeledger/internal/store/fence.go` now carry `type OwnerCheck = storage.OwnerCheck`, a re-export of the one declaration (same pattern as `CostSettler = store.Settler`), because their app layers may not import `storage`. `policy/internal/app`, `authority/internal/app`, `episodes/internal/app`, `episodeledger/internal/app`, the `policy` and `episodeledger` facades and `actions/internal/app/dispatch_test.go` use it.

Ownership fence in episodeledger (safety-relevant):
- `episodeledger/internal/store/fence.go`: `OwnerHoldsLease` (the foreign read of `runtime_owner`) deleted; `Tx.AssertOwner(ctx, owner, epoch)` runs the injected check on the caller's transaction and refuses (error) when the check or the transaction is missing.
- `episodeledger/internal/domain/fence.go`: `ErrOwnerLost`; facade `episodeledger.ErrOwnerLost`.
- `episodeledger/internal/app/{identity,attempt,transition}.go`, `episodeledger/operations.go`: `StartAttemptOwned(..., ownerEpoch, owner, now)` and `TransitionAttempt(..., terminalJSON, owner)` take the check; `ValidateWorkerIdentity` and `validateTerminalIdentity` take it instead of a `now`/`wall` instant (the lease instant is now control's injected clock, no longer `time.Now()` in the facade). `requireOwnerLease` maps a lost owner to `RejectStaleAttempt`, returns any other check error unchanged, and still skips identities without an owner epoch.
- `episodes/internal/store/owner.go` (new), `tx.go`, `lifecycle.go`: `Store.Fenced(check)` binds the owner check; it translates control's `ErrRuntimeOwnerBusy` into `episodeledger.ErrOwnerLost` (so `episodeledger` never imports `control`); `Tx` carries the check into `StartAttemptOwned`/`TransitionAttempt`. `Store.IsFenced`.
- `episodes/config.go`, `episodes/internal/app/service.go`: `ExecutionConfig.RuntimeOwner`; a non-empty `OwnerEpoch` without a runtime owner check is a construction error.
- `runtime/internal/composition/planes.go`: `composeEpisodes` passes `runtimeOwnershipCheck(cfg)`.

"No owner bound" is one decision:
- `runtime/internal/composition/planes.go`: `runtimeOwnershipCheck` is the only place that tests for a missing owner and returns `engine.ReplayOwnership` (the existing no-op check); the duplicate `unownedCheck` is deleted. `policyEpochCheck` uses the same no-op.
- `runtime/internal/composition/pipeline.go`: `NewPipeline` refuses a config with an owner and no epoch or an epoch and no owner (`validateOwnership`), so the unowned mode is "neither" only.
- `runtime/internal/store/owner.go` (new): `assertRuntimeOwner`, the single assertion (refuses when the check is nil, else `runtime ownership lost: %w`). `pipeline.go`, `admission.go`, `costs.go` call it; `PipelineStore.Owner`/`CostConfiguration.Owner` are replaced by `RuntimeOwner storage.OwnerCheck`; the three nil-owner predicates are gone.

Smaller: `control/internal/store/epoch.go` builds `epochControlUpsert` from `domain.EpochKilled` instead of the literal.

Tests updated for new signatures: `episodeledger/{cancellation_fence,lifecycle,ownership}_test.go` and `internal/app/app_test.go` (extra `nil` owner argument for unowned identities; `TestStartingAnAttemptRequiresAnOwnedEpochToHoldTheLease` became `TestStartingAnAttemptRequiresAnOwnedEpoch`), `episodeledger/internal/store/store_test.go`, `runtime/internal/store/pipeline_test.go`, `runtime/internal/app/admission_test.go`, `runtime/reconsideration_test.go` (now pass an explicit check where they relied on a nil owner meaning unowned), `episodes/config_test.go`.

### Decisions

- Injection, not import: `episodeledger` is a package of stateless functions on the caller's transaction, so the check is a parameter of the two owned entry points; `episodes` (which already imports `control`) supplies it. A plain "any error means lost" rule was rejected because an infrastructure failure would then be recorded as a durable stale-attempt rejection; the explicit `ErrOwnerLost` sentinel keeps the old split.
- Fail-closed: a missing check refuses an owned start or transition (it is not treated as unowned); a mismatch of owner and epoch in `NewPipeline` is refused. Previously `Owner == nil` with an epoch was unfenced on every plane except episodes (where the empty `runtime_owner` made it stale); now it cannot be constructed.
- Behaviour changes on purpose: (1) the attempt fence now requires the same `owner_instance` as the epoch holder (control's rule), (2) the lease is judged at the owner's injected clock, not `time.Now()`, (3) `NewPipeline` rejects owner/epoch given alone, (4) runtime-store `AssertOwner` wraps as `assert pipeline owner: runtime ownership lost: ...` (cost configuration now says `runtime ownership lost` instead of `assert owner for cost configuration`; no test matched either), (5) the removed boundary test `lease_boundary_test.go` moved to control.
- `engine.ReplayOwnership` stays the no-op owner check (replay's app layer cannot import `storage`); composition reuses it rather than a second copy.
- Edges: no `allowedImports` or `packageLayers` change.

### Deferred

- `episodeledger/internal/store/dispatch.go` still reads `epoch_control` with the literal `'killed'` (killed-superseded dispatch). Replacing it needs a different query shape (killed epochs read from control and passed in), which risks the dispatch ordering; listed in the follow-ups, not changed here.
- The per-module `Tx.AssertOwner` wrappers keep their own message prefixes (actions, watch, engine, evidence, policy, authority); they add module context and no test matches them.

### Pinning tests

- `TestOwnedStartIsFencedByTheOwnerCheck`, `TestOwnedIdentityIsFencedOnEveryTransition`, `TestOwnerCheckReceivesTheIdentitysEpoch`, `TestCancellationAcknowledgementOfAClosedEpisodeIsStillFenced` (`episodeledger/internal/app/owner_fence_test.go`): lost owner is a stale attempt, other check errors are not rejections, a missing check refuses an owned identity, the check receives the identity's epoch, the closed-episode cancellation path is still fenced.
- `TestOwnedAttemptsAreFencedByTheRuntimeOwnerAssertion`, `TestOwnedTransitionIsRefusedForTheSameEpochFromAnotherInstance` (`episodes/internal/store/owner_fence_test.go`): the same epoch from another instance is refused on start and on transition through control's real assertion; an unfenced store refuses owned attempts.
- `TestHoldsLeaseJudgesEpochInstanceAndInstantTogether` (`control/internal/store/store_test.go`): the lease rule itself (instance, epoch, fractional boundary), replacing `TestOwnerLeaseComparesAsTimeWhenOneSideHasNoFraction`.
- `TestEveryOwnerAssertionGoesThroughTheOneRuntimeOwnerCheck`, `TestOwnerAssertionPassesTheBoundEpoch` (`runtime/internal/store/pipeline_test.go`): pipeline, admission and cost configuration all refuse a lost owner and a missing check, and pass the bound epoch.
- `TestNoOwnerBoundIsDecidedOnceByTheOwnershipCheck`, `TestPipelineRequiresOwnerAndEpochTogether` (`runtime/internal/composition/ownership_test.go`).
- `TestConstructionRejectsIncompleteExecution` (`episodes/config_test.go`): owner epoch without a runtime owner check is refused.
- Existing stand-in "ownership lost" tests for actions, watch, engine and evidence stay green and keep pinning those modules' fences.
