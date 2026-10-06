# Plan

| Round | Scope | Proof | Status |
| --- | --- | --- | --- |
| 0 | Findings, language, design, plan | Review | Complete |
| 1 | Merge `costcontrol` into `control` as one flat package; `CostSettler` port in `episodeledger`; renames; update all callers | Full tests, lint, layer table unchanged except the removed package | Complete |
| 2 | Domain, store, app and facade; layer re-level; ownership and allowlist gates | Layer, ownership and purity gates, tests per layer, lint | Complete |
| 3 | Gates, injection proof, module guide, maps, status docs | Injected failures, full CI, race | Complete |

## Layer re-level (round 2)

`control` domain 1, store 3 (imports `episodeledger`), app 4, facade 5. Importers shift:
authority 4→6, api 5→6, actions store/app/facade 5/6/7→7/8/9, device app/facade 5/6→7/8,
episodes store/app/facade 5/6/7→6/7/8, soak 5→7, executors 8→9, runtime transport 9→10. The exact
table is the output of the longest-path check in the commit.

## Behavior that must not change

Lease claim/renew/release predicates and `ErrRuntimeOwnerBusy` conditions; `AssertDecision*`
and `AssertOrdinaryTx`/`AssertAdmission` refusals and their precedence; kill being terminal; kill
superseding in-flight episodes and releasing only unstarted reservations in the same transaction;
ceiling arithmetic, kill-switch trips on settlement, settlement idempotency; error messages.

## Deliberate changes

- `costcontrol` removed; names per the retired-words table.
- `episodeledger.RecoverUnfinishedAttemptsWithCost` takes a `CostSettler` port instead of the concrete controller.
- (Round 2) the epoch-state string literals become one vocabulary.

## Deferred

- Constructors with errors for `RuntimeOwner`/`EpochControl` (70 literal sites).
- Owner-provided read port for `episodes` used by the unstarted-reservation query.
- Typed epoch state at the database boundary.

## Result (rounds 2 and 3)

- Layers: domain 1, store 3, app 4, facade 5. Moved by the longest-path check: `api` 6, `authority` 6,
  `soak` 7, `actions` store/app/facade 7/8/9, `device` app/facade 7/8, `episodes` store/app/facade 6/7/8,
  executors 9, `runtime` transport 10.
- Gates: `architecture_control_test.go` (facade delegation, opaque store, no raw SQL calls in app) plus
  the generic purity, layering, SQL-location and ownership gates. Each was proven by injecting a violation
  (`os` import and `time.Now` in domain, `database/sql` and a SQL literal in app, logic in the facade, an
  exported store field, store importing app, domain importing storage, `episodeledger` importing control,
  and a foreign writer of `cost_limits`).
- Behavior is preserved: the 25 pre-existing control and cost tests run unchanged against the facade
  (owner claim/renew/release/assert, recovery fencing, kill atomicity and rollback, cost rollback and
  idempotency, ceiling configuration).
- Small deliberate differences: `ErrOwnerNotConfigured` and `ErrEpochControlNotConfigured` are now sentinels
  with the same messages; the supersede call is wrapped as "supersede epoch episodes".
