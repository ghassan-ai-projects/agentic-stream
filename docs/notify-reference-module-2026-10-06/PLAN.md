# Plan

| Round | Scope | Proof | Status |
| --- | --- | --- | --- |
| 0 | Findings, language, design, plan | Review | Complete |
| 1 | Domain (with merged contract), store, app, facade, caller updates and the layer re-level in one commit (the ownership gate would otherwise see two writers) | Layer tests, notify/api/actions/policy/cognition/approvalledger tests, lint | |
| 2 | Architecture gates, injection proof, module guide, maps, status docs | Injected failures, full CI, race | |

## Layer re-level

New: `notify` domain 2, store 2, app 3, facade 4. `notifycontract` is removed.
Importers move: `api` 4→5, `approvalledger` 4→5, `policy/internal/store` 5→6,
`policy/internal/app` 6→7, `policy` 7→8, `runartifact` 8→9,
`runtime/internal/store` 8→9. Actions and cognition stores already sit above 4.

## Behavior that must not change

Error precedence in `Append` (event validation → expiry → canonicalization → duplicate →
tombstone → allocation → insert → race release) and `ReadPage` (limit → oldest cursor
read → highwater read → expiry → lag → page read → per-record poison handling);
sentinel errors and their messages; digests (`canonicaljson` + SHA-256 of the exact
event); gapless cursors on racing identical inserts; audit action names; the seven-day
floor; the three-attempt poison budget; the three independent statements of `Prune`;
contract JSON content.

## Deliberate changes

- `notifycontract` merged into `notify/internal/domain`; goldens and `Types` leave
  production code; contract files move.
- `AppendLifecycleEventWithTrace` → `AppendLifecycleEvent(LifecycleEvent)`.
- `ReadPage` becomes `Service.ReadPage(PageRequest)`; `New(db)` rejects a nil database; the
  SSE handler fails closed (500 stream problem) when its database is missing.
- Audit `details_json` marshalling errors are returned rather than dropped.

## Deferred

- Wire `Prune` into an operator command or runtime job once a retention value is chosen.
- Typed lifecycle payloads; domain-filled `tenant_id`/`source_authority`.
- One transaction for `Prune`; the `tenants`/`tenant` source drift for `situation.trigger.evaluated`.
