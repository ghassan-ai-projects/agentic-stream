# U16 — Quarantine list, release and redrive

Status: todo · Decision: **complete** · Priority: P1 · Size: M · Depends on: U13, U03

## Finding

Invalid or schema-failing evidence goes to `event_quarantine`. The release and
redrive operations exist and are tested (`EventLog.ReleaseQuarantine`,
`RedriveQuarantine`, `ValidRelease`, `Unit.ReleaseQuarantined`,
`ReleasedEnvelope`, `MarkRedriven`: 9 symbols), but nothing calls them. In
production, quarantine is a one-way sink: a valid event that was refused because
its schema was not registered yet can never re-enter the log. Gate A requires
quarantine behavior to be covered; the release status lists redrive tooling as
partial.

## Decision and reasoning

Complete it. Redrive is how an operator recovers evidence after fixing a schema
or a producer. Without it the only options are losing the evidence or editing
SQL.

```text
agentic-stream quarantine list    --db <db> [--status quarantined|released|redriven] [--json]
agentic-stream quarantine show    --db <db> <event-id>
agentic-stream quarantine release --db <db> <event-id> --reason <text>
agentic-stream quarantine redrive --db <db> <event-id>
```

- `release` and `redrive` are owner-fenced (U13). Redrive appends to the event
  log, and the engine must see it the same way it sees live ingress, so it must
  not run beside a live runtime.
- Redrive re-validates against the *current* registered schemas; a still-invalid
  record stays quarantined with its new reason.
- `list` also shows `event_gaps` rows for records whose retries were exhausted,
  which gives that write-only table its first reader.

Keep the two steps (release, then redrive): release is the human decision and
redrive is the mechanical append, which matches the existing state machine and
its tests.

## Done when

- End-to-end test: an event quarantined for an unknown schema is released and
  redriven after the schema is registered, and appears once in the log and the
  Situation history; redriving again is a no-op.
- The 9 eventlog symbols are reachable from `main`.
- `documentation/operations/` has a quarantine runbook; the limitations page
  drops quarantine redrive from "internal capabilities".
