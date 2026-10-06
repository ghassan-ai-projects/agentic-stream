# Cognition migration design

## Layers and ownership

```text
internal/engine, internal/admission
              │
              ▼
internal/cognition                 configured public facade
              │
              ▼
internal/cognition/internal/app    ordered trigger, admission and correction use cases
             ┌┴┐
             ▼ ▼
         domain  store              pure decisions; caller-transaction SQL and adapters
                   │
                   └── storage, scheduleledger, episodeledger,
                       approvalledger, notify
```

Reviewed levels: domain 5, store 6, app 7, facade 8. Domain keeps the existing
`situations.Version` and `spec.Trigger` values, so it depends on the reviewed
Situation layer instead of introducing duplicate projections solely to retain
current numeric levels. The root integration will update the shared import and
layer gates in its own commit.

### Facade

- Own the single `Service` and `New(Config)` construction path.
- Expose `Process(ctx, callerTx, version)` and the cost-refusal transaction
  handoff; delegate immediately to app and store.
- Validate required spec and tenant/deployment identities at construction.
- Retain the physical-clock default. Do not expose evaluation, scheduler,
  SQL, adapter, or mutable engine state.
- Accept the caller's `*sql.Tx` only as an operation input, join it in store,
  and pass only the opaque store transaction inward.

### Application

- Order reads, trigger evaluation, admission, correction reconsideration,
  external ledger handoffs, notifications, and the unconditional last-reasoned
  update.
- Keep all use cases inside the original transaction and preserve current
  operation/error order.
- Depend on `domain` and `store`; do not import `database/sql` or storage.
- Pass the clock into domain decisions and keep every existing clock read at
  the same point in the flow.

### Domain

- Own evaluation and trigger input/result values, condition/score/material
  gate ordering, delta construction, deterministic IDs, capacity decisions,
  and reconsideration eligibility/evidence decisions.
- Keep decisions pure: time and version values arrive as arguments.
- Preserve CEL compilation/evaluation constraints already enforced by the spec
  package. Do not read clocks or storage.

### Store

- Own every cognition SQL statement and the opaque `Tx` joined to the caller's
  original transaction.
- Expose named persistence/projection actions, not `Exec`, `Query`, or raw
  transaction handles.
- Delegate scheduler item changes, episode supersession, approval withdrawal,
  and notifications to their owning modules on the joined transaction.
- Never make admission, correction, or supersession decisions in SQL.

## Public API

| Operation | Caller | Contract |
| --- | --- | --- |
| `New(Config)` | Engine composition | Compile configured triggers once; reject missing spec or tenant/deployment identity. |
| `Service.Process(ctx, tx, version)` | Stream engine | Evaluate and persist cognition atomically with the stream's Situation transaction. |
| `RecordCostRejectionReason(ctx, tx, itemID, refusal)` | Admission | Append the cost refusal to its evaluation in the admission transaction; queue state remains the scheduler ledger's responsibility. |

The old `NewEngine(db, ...)` constructor is retired. Its `db` parameter was
unused; required operations already use caller-owned transactions. The old
`Engine` and public `Scheduler` names are implementation names, not additional
public services.

## Ordered rules that must remain stable

1. Load the last reasoned version and its snapshot; version zero has no prior
   baseline.
2. Evaluate configured triggers in spec order. The first failed gate is
   condition, then material delta, then threshold; compute score after a true
   condition and before material-delta evaluation. A material-delta error
   retains its computed score but records no verdict reason.
3. Persist the evaluation and its notification. Only admitted evaluations
   build scheduler items.
4. Item expiry uses the default 15 minutes when omitted; debounce sets
   not-before; cooldown may move it later based on the prior admitted
   evaluation.
5. At capacity, an existing pending item for the same Situation/trigger can
   be replaced. Otherwise persist a deferred evaluation without a queue item.
6. When admitted, coalesce older pending work, supersede its live episodes,
   cancel their attempts through the episode ledger, announce supersession,
   withdraw superseded approvals, then insert the new scheduler item.
7. For eligible corrections, verify the correction snapshot and persisted
   digest, find invalidated successful actions in command-ID order, deduplicate
   each reconsideration, persist the evidence/evaluation/item/notification,
   and only then advance `last_reasoned_version`.
8. Cost refusal reads and updates evaluation reasons in the caller transaction;
   it does not alter scheduler-item status.

## Existing cross-module reads

Cognition needs projections of Situation history, queue state, and accepted
action outcomes. Its store will contain the SQL during extraction. Separate
owner read ports are a follow-up when each owner can retain the exact supplied
transaction. `scheduleledger`, episode store, and policy store currently read
selected cognition-owned records directly; these reads remain distinct from
mutation ownership and must be considered in the shared architecture review.

## Gate enforcement

The parent integration updates `architecture_test.go`,
`architecture_flow_test.go`, `architecture_ownership_test.go`,
`documentation/architecture/repository-map.md`, and
`documentation/architecture/modules.md`. Required checks: facade delegation,
opaque store transactions, SQL only in cognition store, pure domain, app free
of SQL/storage, strict lower-layer imports, and continued narrow ownership of
`situations.last_reasoned_version`. Each new static gate must be proven by an
injected forbidden source change before removal.

No schema, notification, CEL expression, protobuf, dependency, or golden
contract change is in scope.
