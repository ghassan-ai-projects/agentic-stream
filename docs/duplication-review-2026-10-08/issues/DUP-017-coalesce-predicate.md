# DUP-017: The trigger open-item predicate is selected by cognition and updated by episodeledger as two statements

- Status: fixed
- Severity: low
- Verdict (finders): REAL
- Themes: persistence
- Wave: 2
- Finder sources: P6 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

Use `UPDATE ... RETURNING` in the ledger and delete the cognition select. Keep notification order identical.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report P6: The trigger's open scheduler-item set is selected by cognition and updated by episodeledger with the same hand-copied predicate

- Verdict: REAL
- Shared meaning: "scheduler items of this situation whose trigger evaluation (trigger_name, outcome admitted) is still pending or admitted".
- Sites:
  - internal/cognition/internal/store/supersession.go:75-81 `SELECT scheduler_item_id, situation_version FROM scheduler_items WHERE situation_id=? AND trigger_id IN (SELECT trigger_id FROM trigger_evaluations WHERE situation_id=? AND trigger_name=? AND outcome='admitted') AND status IN ('pending','admitted')`.
  - internal/episodeledger/internal/store/scheduler_queue.go:30-36 `UPDATE scheduler_items SET status='coalesced' ... ` with the identical WHERE.
  - Sequence in internal/cognition/internal/app/supersession.go:18-29: select (LoadSupersession) -> `CoalesceTriggerWork` -> announce the selected items.
  - Related third copy of the "coalesced items of the situation" subquery used to find episodes: internal/episodeledger/internal/store/supersession.go:26-40 (situation-scoped `status='coalesced'`, wider than the trigger-scoped predicate above).
- How they differ: identical today; but the notification set (cognition) and the mutated set (ledger) are two statements that must stay equal; the later episode-supersede step is situation-scoped rather than trigger-scoped (only equal while every coalesced item without an episode has nothing to supersede).
- Risk if left: if one predicate gains a status (e.g. a new pending-like state), items are coalesced but never announced (or announced but not coalesced) in a notification feed that consumers trust.
- Proposed canonical owner: `internal/episodeledger` (writer of `scheduler_items`; cognition store already imports it).
- Proposed fix: make `CoalesceSchedulerItems` return the affected rows via `UPDATE ... RETURNING scheduler_item_id, situation_version` (the repo already uses RETURNING in notify/append.go:62), delete `selectSupersededItemsSQL`, have `LoadSupersession` read only the replacement version. Sort the returned items by id to keep the deterministic order the current `ORDER BY scheduler_item_id` gives.
- Behaviour to preserve: notification order (`situation.superseded:` events, id = situation:version:replacement:item), the ledger status writes, `announceSuperseded` skip of items not older than the replacement.
- Verification: cognition supersession tests (internal/cognition/internal/store/engine_test.go, rollback_test.go) plus a new test asserting notified ids == ids whose status flipped to coalesced.

## Outcome

Found in the tree from the stopped run and checked complete in the Group 2 time rework: `Tx.CoalesceTriggerItems` (`internal/episodeledger/internal/store/scheduler_queue.go`) is one `UPDATE ... RETURNING scheduler_item_id, situation_version`, sorted by item id; cognition's `selectSupersededItemsSQL` is gone and `cognition/internal/store/supersession.go` announces exactly the returned items (`CoalesceTriggerWork`). The Group 2 change made the `now` argument of `CoalesceSchedulerItems` and `SupersedeCoalesced` a `time.Time` (the store formats it), so the facade no longer takes time text.

Pinned by: `TestCoalesceReturnsExactlyTheItemsItFlipped` (`internal/episodeledger/scheduler_lifecycle_test.go`: the returned ids equal the rows whose `updated_at` moved to the coalesce instant, which is now distinct from the seed instant) and the cognition supersession tests.
