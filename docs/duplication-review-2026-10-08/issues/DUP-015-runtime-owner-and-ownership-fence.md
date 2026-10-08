# DUP-015: The runtime_owner lease check is re-implemented in episodeledger and the ownership fence port is declared about ten times

- Status: open
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

Not started.
