# Plan

| Round | Scope | Proof | Status |
| --- | --- | --- | --- |
| 0 | Findings, language, design, plan | Review | Complete |
| 1 | Merge `costcontrol` into `control` as one flat package; `CostSettler` port in `episodeledger`; renames; update all callers | Full tests, lint, layer table unchanged except the removed package | Complete |
| 2 | Domain, store, app and facade; layer re-level; ownership and allowlist gates | Layer, ownership and purity gates, tests per layer, lint | |
| 3 | Gates, injection proof, module guide, maps, status docs | Injected failures, full CI, race | |

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
