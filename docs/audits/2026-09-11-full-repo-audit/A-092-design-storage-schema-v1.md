# A-092 · `docs/design/contracts/storage-schema-v1.sql`

LOC: 1057 · Audit date: 2026-09-11 · Verdict: FIXED

## Bar (close only when every line is true)
- F1: the documented `episodes` state machine matches a real, achievable migration state.
- F2: `decisions`, `episode_events`, `episode_attempts` column sets match cumulative migrations 001+003+010+012+021.
- F3: `situations` columns and CHECK bounds match 001+009.
- F4: every implemented v1 table has a contract definition.
- F5: columns added by later migrations (`002`, `007`, `022`) and contract-listed secondary indexes are present.

## Findings
- **[HIGH] F1. Documented `episodes` lifecycle is a pre/post-cutover chimera that matches no migration state** — `docs/design/contracts/storage-schema-v1.sql:357-402`. The contract shows the pre-003 conflated `status` enum (`:372-388`) and `one_live_episode_per_situation` keyed on `status IN ('accepted','queued','running','cancelling')` (`:400-402`), yet also carries 014's `prompt_sha256`/`objective_sha256` (`:368-369`), which only exist after 003 rebuilt the table with `lifecycle_status`, `current_attempt_id`, `current_fence` (`migrations/003_lifecycle_fencing.sql:33-65,101-103`). It also omits `dispatch_policy`/`policy_epoch` (024) and `stale_rebind_count` (028). The file header requires an ADR for contract changes; DECISIONS.md contains only ADR-001..012 — the breaking cutover was made with neither an ADR nor a contract update. Episodes lifecycle/fencing is release-blocking invariant territory; anyone implementing or validating from this contract builds a different, weaker schema. Fix: regenerate the contract from migrations 001-030 and record the cutover ADR.
- **[MED] F2. `decisions` table is pre-003 shape** — `docs/design/contracts/storage-schema-v1.sql:428-444`. Missing `attempt_id`, `fence`, `rejection_reason` (`migrations/003_lifecycle_fencing.sql:155-174`) and `traceparent`/`tracestate` (`migrations/021_notification_trace_links.sql`). Decision provenance (attempt/fence binding) is invisible in the contract.
- **[MED] F3. `episode_events` is pre-003 shape** — `docs/design/contracts/storage-schema-v1.sql:404-413`. Missing `attempt_id`/`fence` (`migrations/003_lifecycle_fencing.sql:132-143`).
- **[MED] F4. `episode_attempts` drift** — `docs/design/contracts/storage-schema-v1.sql:415-426`. `started_at` is `NOT NULL` in the contract but nullable in 003 (`migrations/003_lifecycle_fencing.sql:122`); `artifact_manifest_json BLOB NOT NULL DEFAULT X'7B7D'` (`:125`) is absent from the contract.
- **[MED] F5. `situations` drift including a CHECK the implemented schema violates by default** — `docs/design/contracts/storage-schema-v1.sql:230-256`. Missing `last_reasoned_version` (`migrations/001_initial.sql:177`); contract says `state_codec_version ... CHECK (>= 1)` (`:246`) but migration 009 adds it as `DEFAULT 0 CHECK (>= 0)` — every freshly migrated row fails the documented constraint.
- **[MED] F6. Sixteen implemented tables have no contract definition** — contract stops around the 018/019 era. Missing: `policy_evaluations` (004), `reconsiderations` (005), `notifications` + `notification_cursors` + `notification_event_tombstones` + `notification_audits` (006), `episode_rejections` + `verifications` (003), `notification_poison_attempts` (020), `intent_dispatch_counts` (022), `epoch_control` (025), `shadow_decisions` (026), `calibration_artifacts` (027), `shadow_comparisons` (029), `device_target_claims`/`device_command_bindings`/`device_authority_events`/`device_reconciliation`/`device_safety_events` (030).
- **[LOW] F7. Column/index drift on otherwise-contracted tables** — `docs/design/contracts/storage-schema-v1.sql` lacks `trigger_evaluations.delta_json` (002), `event_log.tracestate` + `situation_versions.traceparent/tracestate` (007), `intents.rate_limit_per_hour`/`requires_approval` (022), and secondary indexes `event_quarantine_status` (016), `watch_conditions_due` (017), `episode_attempts_live` (003), `episode_attempt_identity` (010), `principals_tenant_status` (013), `cost_limits_tenant` (015), `policy_evaluations_intent_time` (004).

## Checked, not an issue
- S1 (shared tables): the contracted tables, columns, constraints, and indexes match the cumulative migrations through `030`, including trace state, trigger deltas, lifecycle/fencing, governance, cost control, shadow, calibration, and device-authority records.

## Resolution

The contract is now a cumulative snapshot of migrations `001-030` rather than a
pre/post-lifecycle hybrid. It records the lifecycle/fence columns and live
episode index from migration `003`, the attempt artifact and owner columns,
decision provenance and trace links, situation runtime state defaults, trigger
delta, scheduler kind, intent policy columns, and all later tables through the
authority/reconciliation soak records in migration `030`.

The previously missing secondary indexes and cumulative constraints are also
present, including event quarantine, event schema lookup, watch due, attempt
identity/live, policy evaluation, principal, cost-limit, shadow, calibration,
and device-authority indexes.

Evidence: `sqlite3 :memory: < docs/design/contracts/storage-schema-v1.sql`
applies the contract without error. A comparison of the contract against an
in-memory database created by applying every file in `migrations/` found equal
table/index names and equal table column metadata (order, type, nullability,
default, and primary-key position). The isolated change was committed with the
focused spec/schema change as one reviewable contract round.

The migration history remains the executable source of truth. The isolated
write set did not include `docs/design/DECISIONS.md`, so no new ADR entry was
added; migration `003` retains its explicit breaking-cutover record. If the
project requires a separate ADR for that historical decision, it remains a
follow-up outside this isolated change.
- S2: no redundant columns found in the contract's own definitions; keys/uniques/FKs it does declare are correct for the tables it describes.
- S3: not applicable (SQL contract, no domain data).
