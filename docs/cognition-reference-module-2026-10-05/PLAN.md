# Cognition migration plan

## Behavior contract

Preserve trigger order and gate error precedence; trigger identities and
scheduler dedupe bytes; evaluation delta and policy digest bytes; outcome,
reason, lane, and notification names; clock reads and timestamps; debounce,
cooldown, capacity, coalescing, and expiry behavior; correction snapshot
verification and reconsideration identities; exact caller transaction,
cancellation, and rollback behavior; and the narrow
`situations.last_reasoned_version` handoff.

Golden evidence includes `TestTriggerEvaluationPreservesGateOrderAndPartialScore`,
the deterministic ID and scheduler collision tests, engine admission/timing
tests, and reconsideration replay deduplication. Add direct boundary regressions
for caller-transaction rollback, correction digest refusal, and facade/store
encapsulation where existing tests do not prove them.

## Deliberate API changes

- Replace `NewEngine(db, deploymentID, tenantID, spec, idGen, clock)` with
  `New(Config)`. The `db` argument was not used; transaction I/O continues to
  use the caller's transaction. Keep the nil-clock physical default.
- Replace public implementation type `Engine` with opaque `Service` and make
  `Scheduler`, `Evaluation`, and `NewScheduler` private to their owning layers.
- Keep `RecordCostRejectionReason` as a package facade operation because
  admission uses it in a same-transaction lifecycle handoff.
- Reject missing configured spec and tenant/deployment identity at construction
  instead of allowing later panic or malformed writes. Preserve the random ID
  fallback used by the scheduler when no generator is supplied.

## Rounds

| Round | Scope | Proof | Status |
| --- | --- | --- | --- |
| C0 | Survey, canonical language, dated findings/design/plan | Reviewed call graph, data ownership, terms, behavior contract | Complete; ec50ace |
| C1 | Extract pure domain records, deterministic IDs, trigger/delta/CEL and reconsideration decisions | Gate-order/partial-score, digest/identity parity, domain coverage | Complete |
| C2 | Move SQL, storage JSON handling, notification and transaction-owned ledger handoffs to store; make transaction opaque | Same-transaction rollback, foreign handoff ordering, store-only SQL scan | Complete |
| C3 | Move use-case sequencing to app; introduce configured facade; update engine and test callers | Facade-only public surface, constructor safety, unchanged stream/timer/cost paths | Complete |
| C4 | Shared architecture gates, module maps, package guide, audits and final validation | Injected gate failures, full CI, uncached race, diff/docs checks | Final integrated gates pending |

Implementation begins only after the parent commits this C0 plan. Each code
round receives focused tests and review before its commit; the parent owns the
shared gates and commits.

## Code-only/test-only decisions

- Internalize `Scheduler`, `NewScheduler`, and `Evaluation`; they have no
  production callers outside cognition.
- Remove the unused `Engine.idGen` field and unused database constructor input.
- Keep fixture builders in tests. Keep the deterministic-ID collision
  assertion, relocating it to a store/service boundary rather than deleting it.
- Keep the public cost-refusal operation because admission calls it in
  production.

## Follow-up work

1. Replace cognition store's cross-owner outcome joins with owner-provided
   transaction-scoped read ports when episode, policy, and action owners expose
   projections that preserve the caller's transaction.
2. Route the scheduler-ledger coalescing lookup through a cognition-owned
   trigger-ID projection so the lower scheduler-ledger package does not query
   `trigger_evaluations` itself.
3. Add targeted storage fault tests for snapshot decode, notification append,
   and schedule/episode/approval handoff failures beyond the transaction-level
   rollback cases.

These follow-ups do not permit new writers: cognition remains the sole owner of
`trigger_evaluations` and `reconsiderations`, scheduleledger remains owner of
`scheduler_items`, and cognition retains only its documented last-reasoned
Situation handoff.

Missing caller transaction and nil refusal now fail explicitly before I/O.
Implementation is completed directly after stopping the agents. Focused tests,
coverage, lint and injection results are recorded in VALIDATION.md.
