# U22 — Explain a Situation and a trigger decision

Status: todo · Decision: **complete** · Priority: P1 · Size: M · Depends on: U13 (read-only helper)

## Finding

Invariant 10 and the MVP require that every Situation field, trigger decision and
action outcome be explainable from durable records. The records exist: Situation
versions with provenance, `lineage_sets`, trigger evaluations, scheduler items
and `episode_rejections`, policy evaluations, `watch_fires`. Several are
written and never read. TECHNICAL_DESIGN §15 promises `explain situation`,
`explain trigger`, `situation list/show` and matching HTTP endpoints. **None
exist**, so the explainability invariant is held in storage but cannot be
shown to anyone without SQL.

## Decision and reasoning

Complete it as **read-only CLI commands** first, without HTTP endpoints:

```text
agentic-stream situation list    --db <db> [--entity <id>] [--json]
agentic-stream situation show    --db <db> <situation-id> [--version N] [--json]
agentic-stream explain situation --db <db> <situation-id> --version N [--json]
agentic-stream explain trigger   --db <db> <evaluation-id> [--json]
```

- `explain situation` shows, per field, the value, the reducer, and the source
  events (ids, event time, log position) from provenance and lineage.
- `explain trigger` shows the evaluated expression, the delta it saw, the
  outcome (admitted, deferred, coalesced, rejected, canceled, expired), and the
  reason record, which are the outcomes invariant 10 enumerates.
- Each module owns its read query (situations/engine, cognition, episodeledger)
  and exposes it through its facade. The CLI composes them and prints.

Why CLI and not HTTP: TECHNICAL_DESIGN §24 already says a web UI should consume
the HTTP API "after CLI workflows settle". The HTTP API is loopback-only and
token-scoped per surface, so each new route needs its own credential design. CLI
over the database needs none. The design's HTTP endpoints move to deferred
([PLAN_CHANGES](../PLAN_CHANGES.md) P02).

## Done when

- Test on the predictive-maintenance trace: every field of the final Situation
  version traces to at least one event, and every trigger outcome kind present
  in the trace has an explanation with a reason.
- `lineage_sets` and `episode_rejections` have production readers.
- The predictive-maintenance guide ends with an "explain what happened" step.
