# DUP-016: Which pending scheduler items are due is written twice, with different rules, for live and replay

- Status: fixed
- Severity: high
- Verdict (finders): DIVERGED
- Themes: persistence
- Wave: 2b
- Finder sources: P4 (P persistence, R rules, S shapes, M mechanisms)

## Reviewer notes

**Decision (owner, 2026-10-09): enforce `expires_at` on the live path and make replay use the very same selection. One read in `episodeledger` returns due items in live order (not_before, created_at, id) with the expiry and created_at rules applied; live takes the first, replay iterates all. Replay episode ids and goldens change; regenerate them.**

Not dispatched. Live and replay disagree on order, on `created_at` gating and on expiry, and the answer changes deterministic ids. A product decision is needed: enforce `expires_at` live, or drop it from replay. Record it here, then it becomes a fix task.

Finders read the code but ran nothing. The fixer re-reads every site first and corrects or rejects any claim that does not hold, and records that in Outcome.

## Finder reports

### Finder report P4: "Which pending scheduler items are due" is written twice with different rules (live SQL vs replay Go)

- Verdict: DIVERGED
- Shared meaning: choose pending `scheduler_items` that may be admitted to episodes now, and in which order.
- Sites:
  - internal/episodeledger/internal/store/scheduler_queue.go:63-67 live: `status='pending' AND (not_before IS NULL OR not_before <= ?) ORDER BY not_before, created_at, scheduler_item_id LIMIT 1`; used by runtime/internal/store/admission.go:19 -> runtime/internal/app/admission.go:56. No `expires_at` check anywhere on the live path (grep of runtime, episodes, episodeledger: `expires_at` is written and displayed, never compared).
  - internal/replay/internal/store/episodes.go:62-65 replay: `status='pending' ORDER BY scheduler_item_id`, then Go rule in internal/replay/internal/domain/epoch.go:71-107 `AdmissionWindow`/`AdmissionReady`: admitAt = max(created_at, not_before), ready iff `expires.After(admitAt) && !admitAt.After(now)`.
- How they differ / already diverged: (1) order: live by (not_before, created_at, id), replay by id only; (2) expiry: replay refuses items whose expiry is not after their admission time, live never refuses; (3) created_at gating: replay requires `created_at <= now`, live ignores created_at; (4) the scheduler CHECK allows `expired`/`cancelled`/`completed` but nothing writes them, so expiry is unenforced on the live path. This is a live/replay divergence of an admission rule in a product whose invariant is deterministic replay.
- Risk if left: replay admits a different set/order of episodes than the live run; a stale pending item is dispatched live after its `expires_at`.
- Proposed canonical owner: `internal/episodeledger` (owns `scheduler_items`; replay store layer 29 may import it; replay/store does not import episodeledger today, new allowedImports edge `internal/replay/internal/store -> internal/episodeledger`).
- Proposed fix: add one ledger read that returns due items with the window rule applied (e.g. `episodeledger.DueSchedulerItems(tx, tenant, now)` ordered by the live order); live takes the first, replay iterates all; delete `AdmissionWindow/AdmissionReady` from replay domain or move the rule into episodeledger domain. Requires an explicit product decision on expiry (enforce live, or drop from replay) because either way behaviour changes.
- Behaviour to preserve: live admission order for existing runs; replay episode ids (deterministic id generator consumes items in iteration order, so order change alters ids and digests); `expired` is not a written status today.
- Verification: runtime admission tests plus replay golden; new test feeding identical pending items (including `not_before`, expired, created in the future) to both paths and asserting identical selection and order.

## Outcome

Found in the tree from the stopped run and checked complete in the Group 2 time rework: `episodeledger.DueSchedulerItems` (one read, `domain.DueItems` in `internal/episodeledger/internal/domain/queue.go`) returns due items in live order (not_before with none first, created_at, id) with the created_at and expiry window applied. Live takes the first item that is not stale at now (`NextPendingSchedulerItem`, used by `runtime/internal/store/admission.go`); replay iterates all of them (`replay/internal/store/episodes.go`) and persists each at its admission instant. Replay's own `AdmissionWindow`/`AdmissionReady` are gone. Expiry is now enforced on the live path (owner decision). Replay episode ids follow the new iteration order; replay tests pass with no golden left to regenerate in the replay tree.

The Group 2 change only moved the codec: the queue store parses `created_at`, `not_before` and `expires_at` once with `kernel.ParseTime` and fails closed with a wrapped error naming the item and column; the rule works on `time.Time` with stdlib comparisons.

Pinned by: `TestDueItemsAppliesWindowRulesInQueueOrder`, `TestDueItemAdmitAtIsTheLaterOfCreationAndNotBefore`, `TestDueItemIsStaleFromItsExpiryInstant` (domain), `TestLiveAdmissionOrderEqualsTheReplaySelection`, `TestLiveAdmissionSkipsItemsPastTheirExpiry` (`internal/episodeledger/scheduler_lifecycle_test.go`).
