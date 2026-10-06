# Plan

| Round | Scope | Proof | Status |
| --- | --- | --- | --- |
| 0 | Findings, language, design, plan | Review | Complete |
| 1 | Domain (with merged contract), store, app, facade, caller updates and the layer re-level in one commit (the ownership gate would otherwise see two writers) | Layer tests, notify/api/actions/policy/cognition/approvalledger tests, lint | Complete |
| 2 | Architecture gates, injection proof, module guide, maps, status docs | Injected failures, full CI, race | Complete |
| 3 | Typed lifecycle payloads: one struct per event type, tenant and source authority stamped by the domain, all eight producers converted, policy's approval notification typed | Golden reproduction per type, JSON-shape regression test, full CI | Complete |

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
- One transaction for `Prune`; the `tenants`/`tenant` source drift for `situation.trigger.evaluated`.

## Gates added (round 2)

`architecture_notify_test.go`: facade only delegates, store `Store`/`Tx` fields private,
app uses no raw SQL calls. Existing generic gates now also cover notify: pure domain,
no `database/sql` in app, SQL only in store, table ownership (`internal/notify/internal/store`),
layer table, import allowlists. Each was proven by injecting a violation (clock read in
domain, `database/sql` in app, SQL literal in app, logic in facade, exported store field,
domain importing storage, store importing domain, notify importing api, a foreign writer of
`notifications`) and watching it fail.

## Round 3: typed lifecycle payloads

- `LifecycleEvent` carries a `Payload` whose type fixes the event type; the `Type` field and the
  `Type*` facade constants are gone, so a type and its data cannot disagree.
- `tenant_id` and `source_authority` are no longer written by producers; the domain stamps both
  from the envelope, so the binding rule can no longer be violated by a producer.
- Payload structs have exactly the contract's fields; the contract test decodes every golden event
  into its struct with unknown fields refused and compares the produced data with the golden.
- `policy` seals an `ApprovalNotification` struct (same canonical JSON as the old map; a nil `delta`
  stays `{}`, covered by a regression test) and its store checks the notification is bound to the
  row's tenant before publishing.
- Still untyped: enum-like strings (`status`, `verdict`, `risk_class`) are validated by the schema, not
  by Go types; `ApprovalRequested.Delta` and `.Action` are open objects by contract.
