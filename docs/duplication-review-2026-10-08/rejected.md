# Looked at and rejected

Each finder listed what it checked and discarded, so the ground is not covered twice. A rejection here is a finding too: re-open it only if the code changes.

## Persistence

- `situation_versions.snapshot_sha256` point lookups (cognition/store/reconsideration.go:117, policy/store/approval_context.go:16, replay/store/store.go:77,85, episodes/store/assembler.go:48) and `situations.current_version` (episodes assembler.go:61, cognition supersession.go:46): REAL duplicates, but the owner is `engine` (facade layer 25) and cognition (20), policy store (20), episodes store (21), replay (29 store but its domain layer) cannot import it upward; fixing needs a new low-layer reader package, not worth it for 6 one-line reads. Revisit only if situation_versions columns change.
- Expiry rule "unparseable or not after now = expired" (actions/domain/authorization.go:81,102, dispatch.go:23, candidate.go:34; policy/domain/rules.go:57, routing.go:36; decisions/domain/intent.go:105, validator.go:157; watch/domain/condition.go:67): same fail-closed rule in 8 places, all equivalent; a `sources.ExpiredAt(text, now)` is a clean small fix but belongs to the domain-rules theme. decisions deliberately rejects unparseable with `schema_invalid` rather than `expired` (intentional).
- `IN (?, ?, ...)` placeholder builders: actions/store/unresolved.go:38, authority/store/events.go:52, engine/store/timers.go:90 (three tiny copies, no `storage.Placeholders`); trivial, fold into C2/C3 fixes if an `IN` list is generated.
- `sql.NullString` trace pair scanned into `contractsv1.TraceContext`: 15 sites (cognition, actions, approvalledger, engine, policy, eventlog); real boilerplate, low risk; one `storage`/`contractsv1` scan helper would remove it.
- actions outbox close variants: outcomes.go:22 (`closeDispatchOutboxSQL`), candidates.go:84 (fail with code), candidates.go:93 (close only) each rewrite `status, lease_owner=NULL, lease_until=NULL, delivered_at=CASE ...`; intra-module, one `closeOutbox` statement would do. Command status transitions (`pending|failed|dispatching`->dispatching, `dispatching`->X, `reconciling|outcome_unknown|manual_review`->X, any->failed at candidates.go:81 unconditional) exist only as WHERE clauses; the command state machine has no domain transition table (unlike episodes attempts). Also `actions/domain/reconciliation.go:124` returns the literal "verified" while the verification constants block lacks it.
- `event_quarantine` retry cap `10` hard-coded three times in one SQL statement (eventlog/store/quarantine.go:46,49): single module, single statement.
- Hash-of-pipe-joined-string identities (`episodes/domain/assembly.go:100`, `policy/domain/routing.go:79`, `policy/domain/commands.go:13`, `cognition/domain/timing.go:67`, `correction.go:77`, `episodeledger/domain/rejection.go:103`, `engine/domain/heartbeat.go:94`): each has distinct material and prefix; same idiom, not the same rule.
- Leases: `outbox.lease_*` (actions), `runtime_owner` (control), `evidence_call_ledger.lease_*`, `device_target_claims.lease_until` (authority) are four lease implementations with different fencing contracts; not a shared rule.
- Per-module `Tx`/`Join` types, `notify`, `watch`, `spec`, `ingress`, `interlock`, `storage` store SQL: single-owner tables, no cross-module reads beyond `event_schemas` (eventlog reads `schema_json WHERE status='active'`, spec reads the digest without the status filter - different purposes).
- `policy_evaluations.result` and `intents.policy_status` CHECK lists (migrations 001/004) share six values; written by the same policy path from one `Outcome.Status`; no second writer. Policy status words are raw literals in policy, actions (`approved`) and cognition SQL (reconsideration.go:94) but have no divergent set.
- `runartifact` `SELECT *` ledger dumps and `migrations` snapshot contract (ADR-014): deliberate schema-bound exports.
- Nothing found to flag in cursor/pagination conventions beyond C1/C4: `notifications.cursor`, `event_log.position`, `outbox_id` are each single-owner monotonic keys.

## Business rules and constants

- `time.Duration` parsing ("7d"): all callers already go through `spec.ParseDuration` (operators, cognition, episodes, engine, situations); consolidated.
- Trace context validation: centralized in `contractsv1.ParseTraceContext`; every caller uses it.
- Tenant-id empty checks (notify, evidence, ingress, cognition, runartifact, eventlog, control): one-line `== ""` guards with module-specific error text; no shared rule.
- Notification retention 168h: enforced once (`notify.CheckRetention`); cmd only mentions "168h" in help text (cosmetic).
- `notify.MaxPageLimit = 1000` vs `api.NormalizePageSize` (`size > 1000 -> 100`, internal/api/internal/domain/stream.go:44-49): same number, but different behaviour (reject vs clamp-to-default) and API vs outbox concerns; borderline, left out.
- Watch limits (`MaxFires <= 100`, expression length 4096): defined only in watch/domain/condition.go:56-57 (the aquaculture catalog schema carries its own copy as data); single Go site.
- Quarantine `attempt_count >= 10` (eventlog store SQL) and `maxSeenBootIDs = 64`, `globalCapacity = 100`, `maxEpisodeAttempts = 3` / `maxStaleRebinds = 3`: each defined once; the two `3`s are different concepts.
- Synthetic Decision/Intent documents (fixture executor.go:61-102, native deterministic.go:27, replay baseline.go:117-160) share a shape and a far-future expiry literal ("2099-01-01T00:00:00.000000000Z" vs "2099-01-01T00:00:00Z", which gives different intent digests); these are fixture/baseline producers, not business rules, so not ranked here - worth a later look if a Decision builder is wanted in `decisions`.
- cmd/agentic-stream and internal/api: no SQL, no lifecycle/risk/status decisions beyond C7 and the lease literals in C9.
- Command/outbox/outcome/verification vocab in actions: single owner (`actions/internal/domain/status.go`), SQL literals inside the same module only (aside from C4).
- Engine timers / situation / watch / notification status strings: each confined to its owning module and store.
- `riskRank` equivalence for valid inputs: the two functions order R1..R4 identically; only R0/unknown and the numbering differ (covered in C2).
- Tests, fixtures and generated protobuf code were excluded.

## Contracts and shapes

- Notification lifecycle type list (Go `lifecycleTypes` vs JSON enum and `if/then` blocks in `notification-contract-v1.json`): duplication is by design (contract-first schema) and `lifecycle_contract_test.go` pins counts against goldens.
- `CommandView`/`OutcomeView` (actions), `IntentView` (policy), `DecisionView` (episodes), `EpisodeRecord` (episodeledger), `ApprovalView`, `WatchView`: each facade only aliases the app/domain view; they are distinct owned projections and cmd composes them without copying fields (intent_command.go, episode_command.go).
- `policy.IntentRecord` vs `actions.IntentRow/DecisionRow/EpisodeRow/SituationRow`: different SELECT shapes owned by different modules; only the verification logic duplicates (reported as C3), not the row types.
- Evidence request fingerprint and result hash (`evidence/wire/fingerprint.go`, `result.go`): single writer and single verifier per record; the extra raw `sha256` sites are folded into C9.
- Device wire records (`device/internal/wire`) vs `contractsv1` device schemas: validation happens once through `contractsv1.Validate` before typed parsing; no second validator.
- Error sentinels: all exported sentinels are single-definition; text matching exists only in the two SQLite matchers (C11). No cross-module `err.Error()` comparison elsewhere.
- Per-module `Tx`/`Join` types, `DecisionRecord` vs `DecisionView`, `AttemptRecord` vs `AttemptView`: intentional separation (write record vs read view).
- `Canonicaljson` domain list and prefixes (`situation-runtime/<kind>/v1\n`): single definition; only usage sites vary (C6).
- internal/api approval input (`ApprovalInput`) vs policy `ApprovalAssertion`: HTTP body vs signed assertion are different documents; api builds no digest.
- Evidence `EventRecord` vs eventlog `Record`: evidence projects eventlog-owned reads into a closed result envelope on purpose.
- `cmd` text rendering helpers (`orNone`, `compactValue`, `versionHeader`): presentation only, single copy.

## Mechanisms

- WithTx/InTx wrappers in notify, evidence, episodes, actions, watch, engine, control, authority (store.go/tx.go in each): every one is `s.db.WithTx(ctx, func(tx *sql.Tx) error { return work(Join(tx)) })` over `storage.DB.WithTx` (storage/internal/store/storage.go:174), which has the only begin/rollback/commit; no isolation, retry or rollback differences. Intentional per-module opaque Tx (AGENTS.md).
- Differences that look like WithTx variants but are intentional: `Autocommit()` in notify/control (statement outside a tx), runartifact `BeginTx(ReadOnly)`, runtime `PipelineStore.AssertOwner` opening its own tx (listed under C3 only as a divergence).
- applicationConfig x5 / validateConfig x3: each validates different required parts and maps different fields; the only shared piece (tenant default) is C8, the clock defaults are C9.
- `orMinute`/`ReservationLease` lease default: already consolidated into `sources.OrLease`; remaining lease-default copies are in C8.
- Trace context: validation is already centralised in `contractsv1.ParseTraceContext` (used by eventlog, evidence, episodes, notify, cognition, workerfake); remaining items are cosmetic - ten structs carry `Traceparent, Tracestate string` pairs and several sites rebuild `contractsv1.TraceContext{...}`; `TraceContext.Link()`/`SpanLink` (contractsv1/internal/domain/trace_context.go:57) is unused beside `telemetry.AddLinkFromW3C`; notify lifecycle_contract.go:84 re-states "tracestate requires traceparent" after `ParseTraceContext` already enforced it (redundant, not divergent).
- Telemetry/span registration: only three spans (`episode.execute`, `worker.execute`, `pipeline.batch`) with distinct meanings; metrics live in one `telemetry.Runtime`.
- Replay read-only DSN (`replay/internal/transport/database.go:51`, busy_timeout 5000) vs `storage.ConnectionString`: read-only snapshot connection, deliberately separate.
- Per-plane composition in runtime/internal/composition/planes.go and cmd (`openRuntimeCore`, `serve`, `run-live`): the per-plane `compose*` functions differ in config; run-live and serve already share `runtimeCore`, `liveFlags.registerShared`, `cleanups`. Only the owner-check closure and defaults (C3, C8) repeat.
- Native `"dec_"` literal (executor/native/internal/domain/deterministic.go:27) vs `sources.PrefixDecision`: the native domain may not import sources (known: FOLLOW_UPS #9).
- Inbox/outbox recovery sweepers (evidence `ReclaimExpired`/`Recover`, episodeledger `RecoverUnfinishedAttempts`, actions outbox lease expiry in `candidates.go`, watch `ExpireDue`, notify `Prune`): each sweeps a different table with a different rule (epoch mismatch vs lease expiry vs retention); only the time-ordering concern is shared (C1) and the retry concern (C7).
- Epoch drain/kill handling: single implementation in control/internal/domain/epoch.go (`RefuseDecision`, `RefuseOrdinary`, `RefuseAdmission`) used through the control facade; only the SQL literals `'killed'` outside control repeat (folded into C2).
- `device/internal/app/session_reconcile.go:93` predicate (`ErrRuntimeOwnerBusy || ErrEpochKilled || ErrEpochDraining`): a single caller, not repeated elsewhere.
- Row-collection loops (`rows.Next()` in control, engine timers, replay, runartifact, approvalledger, notify, eventlog, episodeledger): belongs to the storage-helpers theme (`storage.CollectRows` exists); not re-reported here.

