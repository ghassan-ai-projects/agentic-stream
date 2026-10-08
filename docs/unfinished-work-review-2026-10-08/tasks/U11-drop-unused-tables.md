# U11 — Drop unused tables

Status: done · Decision: **drop two, keep one as reserved** · Priority: P3 · Size: S · Depends on: U02

## Finding

A scan of every `CREATE TABLE` in `migrations/` against production Go code:

| Table | Writers | Readers | Origin |
| --- | --- | --- | --- |
| `replay_jobs` | 0 | 0 | `001_initial.sql`, for `POST /v1/replays` (never built) |
| `episode_events` | 0 | 0 | `001_initial.sql`, rebuilt in `003`; the design §12.2 episode event log, never persisted |
| `artifacts` | 0 | 0 | `001_initial.sql`, the §14.3 artifact store (never built, see U05); referenced by FK columns `original_artifact_id` and `replay_jobs.result_artifact_id` |

Tables that are written but never read (`event_gaps`, `lineage_sets`,
`episode_rejections`, `watch_fires`, `notification_audits`, `shadow_decisions`,
`shadow_comparisons`) are audit records. They get readers in U16, U21, U22 and
U23. Tables read but never written (`principals`, `roles`, `principal_roles`,
`approval_authorities`, `calibration_artifacts`) are handled in U15 and in
[PLAN_CHANGES](../PLAN_CHANGES.md) P06.

## Decision and reasoning

- **Drop `replay_jobs` and `episode_events`** in a new migration. Replay runs
  in an isolated database by design (§16.2), so a production table of replay
  jobs contradicts the design. Episode events are streamed to the runtime and
  summarized in attempts and decisions, and nothing reads them back. Empty
  tables in the schema mislead readers about what is persisted.
- **Keep `artifacts`** for now and mark it reserved in
  `documentation/contracts/persistence.md`. Dropping it requires rebuilding the
  table that holds `original_artifact_id`, which is a riskier migration for no
  behavior gain. Revisit it with the next migration that rebuilds that table.

## Done when

- A new migration drops the two tables; the storage schema-version test is
  updated; `docs/design/contracts/storage-schema-v1.sql` matches.
- A fresh database and an upgraded database have the same schema.
