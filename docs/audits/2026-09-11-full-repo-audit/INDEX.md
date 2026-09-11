# Full Repository Audit — 2026-09-11

Scope: top 100 largest code files (Go, SQL, proto, embedded schema/data JSON).
Generated protobuf code is listed but skipped.

Status legend: `[ ]` pending · `[x]` done. Verdict legend:
- **PASS** — meets the bar. No audit file.
- **FINDINGS** — improvement or refactor required. Audit file exists: `A-NNN-<name>.md`.

Fix legend (implementation phase, tracked in the item line):
- `✅ FIXED` — findings implemented, package tests pass, committed. See the finding file's Resolution section.
- `🔧 WIP` — an agent has claimed this item and is actively working it (do not double-work).
- (no marker) — findings not yet started.

> Two agents work this audit concurrently. Claim an item by marking it `🔧 WIP` before editing source, and re-read the item line before touching it.

## The Bar — production files

A production file passes only if all hold:

- **P1 Correctness.** Errors wrapped with `%w`, none swallowed. Contexts honored. No data races; mutex discipline explicit.
- **P2 Safety/invariants.** Untrusted input never becomes instructions or executable parameters. Effector access only via policy revalidation + action plane. Replay performs no external effects.
- **P3 Simplicity.** No dead code. No unused exported symbols. No speculative abstraction ("for future flexibility"). No duplicated logic that can diverge. Functions focused; long functions justified.
- **P4 Structure.** Package boundaries match design §23. Dependencies flow downward. No transport concerns in the engine. No per-domain branches in code (domains are data).
- **P5 Consistency.** `log/slog` logging, `context.Context` first parameter, exported symbols documented, stdlib helpers (`slices`/`maps`/`cmp`), canonical JSON (RFC 8785) wherever a digest is computed.
- **P6 Tests.** Package has meaningful tests for its behavior; no tautological tests; modified behavior covered.
- **P7 Determinism.** Serial state changes per virtual partition; stable identities; idempotency on cross-boundary effects.

## The Bar — test files

A test file passes only if all hold:

- **T1 Real assertions.** No tautologies; assertions check behavior, not implementation trivia.
- **T2 Deterministic.** No network, no sleeps/timing races, parallel-safe where safe.
- **T3 Idiomatic.** Table-driven with `t.Run()` where natural; `t.Context()` where appropriate.
- **T4 Coverage value.** Covers behavior and edge cases, not line-count padding; no brittle over-coupling.
- **T5 Hygiene.** No copy-paste blocks masking a missing helper; fixtures not duplicated across files.

## Data/schema bar

- **S1** Schema/SQL/proto matches implemented behavior and migrations; no drift.
- **S2** No redundant columns/tables/fields; constraints enforce invariants.
- **S3** Domain data stays in data files, never re-authored in Go (digest pins respected).

## Index

### Generated (skipped)

- [x] G-001 `proto/agenticstream/runtime/v1/runtime-v1.pb.go` (3292) — GENERATED, protoc output. Never hand-edit. SKIP.
- [x] G-002 `proto/agenticstream/runtime/v1/runtime-v1_grpc.pb.go` (277) — GENERATED, protoc output. Never hand-edit. SKIP.

### Production files

- [x] A-001 `internal/replay/replay.go` (1003) — FINDINGS → [A-001-replay-replay.md](A-001-replay-replay.md)
- [x] A-002 `internal/actions/dispatcher.go` (892) — FINDINGS → [A-002-actions-dispatcher.md](A-002-actions-dispatcher.md)
- [x] A-003 `internal/episodes/executor.go` (851) — FINDINGS · HIGH → [A-003-episodes-executor.md](A-003-episodes-executor.md)
- [x] A-004 `internal/storage/authority.go` (836) — FINDINGS → [A-004-storage-authority.md](A-004-storage-authority.md)
- [x] A-005 `internal/operators/operators.go` (778) — FINDINGS · HIGH → [A-005-operators-operators.md](A-005-operators-operators.md)
- [x] A-006 `internal/episodes/assembler.go` (764) — FINDINGS → [A-006-episodes-assembler.md](A-006-episodes-assembler.md)
- [x] A-007 `internal/episodes/worker_executor.go` (682) — FINDINGS → [A-007-episodes-worker_executor.md](A-007-episodes-worker_executor.md)
- [x] A-008 `cmd/agentic-stream/main.go` (648) — FINDINGS → [A-008-cmd-main.md](A-008-cmd-main.md)
- [x] A-009 `internal/runtime/pipeline.go` (646) — FINDINGS → [A-009-runtime-pipeline.md](A-009-runtime-pipeline.md)
- [x] A-010 `internal/canonicaljson/canonicaljson.go` (624) — PASS
- [x] A-011 `internal/situations/situations.go` (597) — FINDINGS → [A-011-situations-situations.md](A-011-situations-situations.md)
- [x] A-012 `internal/executor/native/native.go` (560) — FINDINGS · HIGH → [A-012-executor-native-native.md](A-012-executor-native-native.md)
- [x] A-013 `internal/decisions/validator.go` (510) — FINDINGS · HIGH → [A-013-decisions-validator.md](A-013-decisions-validator.md)
- [x] A-014 `internal/cognition/scheduler.go` (479) — FINDINGS → [A-014-cognition-scheduler.md](A-014-cognition-scheduler.md)
- [x] A-015 `internal/cognition/engine.go` (446) — FINDINGS → [A-015-cognition-engine.md](A-015-cognition-engine.md)
- [x] A-016 `internal/ingress/live_socket.go` (423) — FINDINGS → [A-016-ingress-live_socket.md](A-016-ingress-live_socket.md)
- [x] A-017 `internal/eventlog/log.go` (419) — FINDINGS → [A-017-eventlog-log.md](A-017-eventlog-log.md)
- [x] A-018 `internal/episodes/lifecycle.go` (401) — FINDINGS → [A-018-episodes-lifecycle.md](A-018-episodes-lifecycle.md)
- [x] A-019 `internal/ingress/simulator.go` (393) — FINDINGS · HIGH → [A-019-ingress-simulator.md](A-019-ingress-simulator.md)
- [x] A-020 `internal/actions/watch_effector.go` (379) — FINDINGS → [A-020-actions-watch_effector.md](A-020-actions-watch_effector.md)
- [x] A-021 `internal/spec/compiler.go` (349) — FINDINGS → [A-021-spec-compiler.md](A-021-spec-compiler.md)
- [x] A-022 `internal/telemetry/runtime.go` (338) — FINDINGS → [A-022-telemetry-runtime.md](A-022-telemetry-runtime.md)
- [x] A-023 `internal/actions/uds_transport.go` (319) — FINDINGS → [A-023-actions-uds_transport.md](A-023-actions-uds_transport.md)
- [x] A-024 `internal/notify/notify.go` (317) — FINDINGS → [A-024-notify-notify.md](A-024-notify-notify.md)
- [x] A-025 `internal/actions/serial_materialize.go` (315) — PASS
- [x] A-026 `internal/worker/server.go` (314) — FINDINGS → [A-026-worker-server.md](A-026-worker-server.md)
- [x] A-027 `internal/actions/serial_session_state.go` (310) — FINDINGS → [A-027-actions-serial_session_state.md](A-027-actions-serial_session_state.md)
- [x] A-028 `internal/policy/policy_command.go` (300) — FINDINGS → [A-028-policy-policy_command.md](A-028-policy-policy_command.md)
- [x] A-029 `internal/evidence/ledger.go` (298) — FINDINGS → [A-029-evidence-ledger.md](A-029-evidence-ledger.md)
- [x] A-030 `internal/engine/engine_state.go` (288) — FINDINGS · HIGH → [A-030-engine-engine-state.md](A-030-engine-engine-state.md)
- [x] A-031 `internal/runtime/worker_runtime.go` (286) — FINDINGS → [A-031-runtime-worker_runtime.md](A-031-runtime-worker_runtime.md)
- [x] A-032 `internal/runartifact/export_snapshot.go` (286) — FINDINGS → [A-032-runartifact-export_snapshot.md](A-032-runartifact-export_snapshot.md)
- [x] A-033 `internal/engine/engine_timers.go` (286) — FINDINGS → [A-033-engine-engine-timers.md](A-033-engine-engine-timers.md)
- [x] A-034 `internal/evidence/capability.go` (284) — FINDINGS → [A-034-evidence-capability.md](A-034-evidence-capability.md)
- [x] A-035 `internal/spec/spec.go` (272) — FINDINGS → [A-035-spec-spec.md](A-035-spec-spec.md)
- [x] A-036 `internal/notify/sse.go` (260) — FINDINGS → [A-036-notify-sse.md](A-036-notify-sse.md)
- [x] A-037 `internal/evidence/server.go` (255) — PASS
- [x] A-038 `internal/executor/native/openai.go` (242) — FINDINGS · HIGH → [A-038-executor-native-openai.md](A-038-executor-native-openai.md)
- [x] A-039 `internal/actions/serial_session_exchange.go` (237) — FINDINGS → [A-039-actions-serial_session_exchange.md](A-039-actions-serial_session_exchange.md)
- [x] A-040 `internal/actions/serial_session_safety.go` (226) — FINDINGS → [A-040-actions-serial_session_safety.md](A-040-actions-serial_session_safety.md)
- [x] A-041 `internal/policy/policy_evaluate.go` (225) — FINDINGS · HIGH → [A-041-policy-policy_evaluate.md](A-041-policy-policy_evaluate.md)
- [x] A-042 `internal/actions/serial_session.go` (215) — PASS
- [x] A-043 `internal/engine/engine_events.go` (213) — FINDINGS → [A-043-engine-engine-events.md](A-043-engine-engine-events.md)
- [x] A-044 `internal/eventlog/quarantine.go` (209) — FINDINGS → [A-044-eventlog-quarantine.md](A-044-eventlog-quarantine.md)
- [x] A-045 `internal/runartifact/export_verify.go` (200) — FINDINGS → [A-045-runartifact-export_verify.md](A-045-runartifact-export_verify.md)
- [x] A-046 `internal/storage/storage.go` (195) — FINDINGS → [A-046-storage-storage.md](A-046-storage-storage.md)
- [x] A-047 `internal/replay/baseline.go` (189) — FINDINGS → [A-047-replay-baseline.md](A-047-replay-baseline.md)
- [x] A-048 `internal/costcontrol/costcontrol.go` (185) — PASS
- [x] A-049 `internal/ingress/jsonl.go` (183) — FINDINGS · HIGH → [A-049-ingress-jsonl.md](A-049-ingress-jsonl.md)
- [x] A-050 `internal/policy/policy.go` (182) — FINDINGS → [A-050-policy-policy.md](A-050-policy-policy.md)
- [x] A-051 `internal/engine/engine_restore.go` (180) — FINDINGS → [A-051-engine-engine-restore.md](A-051-engine-engine-restore.md)
- [x] A-052 `internal/storage/runtime_owner.go` (178) — PASS → [A-052-storage-runtime-owner.md](A-052-storage-runtime-owner.md) (PASS record)
- [x] A-053 `internal/actions/serial_effector.go` (178) — FINDINGS → [A-053-actions-serial_effector.md](A-053-actions-serial_effector.md)
- [x] A-054 `internal/policy/policy_approval.go` (177) — FINDINGS → [A-054-policy-policy_approval.md](A-054-policy-policy_approval.md)
- [x] A-055 `internal/soak/report_compute.go` (176) — PASS
- [x] A-056 `internal/cognition/reconsideration.go` (169) — FINDINGS → [A-056-cognition-reconsideration.md](A-056-cognition-reconsideration.md)
- [x] A-057 `cmd/agentic-stream/effect_profile.go` (165) — PASS
- [x] A-058 `internal/clock/clock.go` (164) — FINDINGS · HIGH → [A-058-clock-clock.md](A-058-clock-clock.md) — ✅ FIXED
- [x] A-059 `internal/notifycontract/contract.go` (161) — FINDINGS → [A-059-notifycontract-contract.md](A-059-notifycontract-contract.md)
- [x] A-060 `internal/worker/uds.go` (159) — FINDINGS → [A-060-worker-uds.md](A-060-worker-uds.md)
- [x] A-061 `internal/runartifact/export.go` (158) — FINDINGS → [A-061-runartifact-export.md](A-061-runartifact-export.md)
- [x] A-062 `internal/storage/epoch_control.go` (149) — FINDINGS → [A-062-storage-epoch-control.md](A-062-storage-epoch-control.md)
- [x] A-063 `internal/engine/engine_apply.go` (147) — FINDINGS → [A-063-engine-engine-apply.md](A-063-engine-engine-apply.md)
- [x] A-064 `internal/episodes/intent_catalog.go` (132) — PASS
- [x] A-065 `internal/policy/policy_store.go` (131) — PASS
- [x] A-066 `internal/runtime/service.go` (130) — PASS
- [x] A-067 `internal/episodes/recovery.go` (124) — FINDINGS → [A-067-episodes-recovery.md](A-067-episodes-recovery.md)
- [x] A-068 `internal/runartifact/export_verify_ledger.go` (116) — FINDINGS → [A-068-runartifact-export_verify_ledger.md](A-068-runartifact-export_verify_ledger.md)
- [x] A-069 `internal/ids/ids.go` (114) — FINDINGS → [A-069-ids-ids.md](A-069-ids-ids.md)

### Test files

- [x] A-070 `internal/cognition/engine_test.go` (1316) — FINDINGS → [A-070-cognition-engine_test.md](A-070-cognition-engine_test.md)
- [x] A-071 `internal/episodes/assembler_test.go` (1140) — FINDINGS → [A-071-episodes-assembler_test.md](A-071-episodes-assembler_test.md)
- [x] A-072 `internal/operators/operators_test.go` (677) — FINDINGS · HIGH → [A-072-operators-operators_test.md](A-072-operators-operators_test.md)
- [x] A-073 `internal/episodes/rebind_test.go` (584) — FINDINGS → [A-073-episodes-rebind_test.md](A-073-episodes-rebind_test.md)
- [x] A-074 `internal/actions/dispatcher_test.go` (572) — FINDINGS → [A-074-actions-dispatcher_test.md](A-074-actions-dispatcher_test.md)
- [x] A-075 `internal/replay/replay_test.go` (519) — FINDINGS → [A-075-replay-replay_test.md](A-075-replay-replay_test.md)
- [x] A-076 `internal/actions/serial_effector_test.go` (492) — FINDINGS → [A-076-actions-serial_effector_test.md](A-076-actions-serial_effector_test.md)
- [x] A-077 `internal/decisions/validator_test.go` (436) — FINDINGS → [A-077-decisions-validator_test.md](A-077-decisions-validator_test.md)
- [x] A-078 `internal/engine/engine_test.go` (427) — FINDINGS · HIGH → [A-078-engine-engine_test.md](A-078-engine-engine_test.md)
- [x] A-079 `internal/policy/policy_test.go` (359) — PASS
- [x] A-080 `internal/storage/storage_test.go` (357) — FINDINGS · HIGH → [A-080-storage-storage_test.md](A-080-storage-storage_test.md)
- [x] A-081 `internal/eventschema/registry_test.go` (354) — FINDINGS → [A-081-eventschema-registry_test.md](A-081-eventschema-registry_test.md)
- [x] A-082 `internal/spec/compiler_test.go` (323) — FINDINGS → [A-082-spec-compiler_test.md](A-082-spec-compiler_test.md)
- [x] A-083 `internal/actions/phase04_test.go` (333) — FINDINGS → [A-083-actions-phase04_test.md](A-083-actions-phase04_test.md)
- [x] A-084 `internal/episodes/worker_executor_test.go` (348) — PASS
- [x] A-085 `internal/episodes/p8_freshness_test.go` (348) — FINDINGS → [A-085-episodes-p8_freshness_test.md](A-085-episodes-p8_freshness_test.md)
- [x] A-086 `internal/actions/serial_session_test.go` (348) — PASS
- [x] A-087 `internal/actions/uds_transport_test.go` (343) — FINDINGS → [A-087-actions-uds_transport_test.md](A-087-actions-uds_transport_test.md)
- [x] A-088 `internal/episodes/runner_test.go` (311) — FINDINGS → [A-088-episodes-runner_test.md](A-088-episodes-runner_test.md)
- [x] A-089 `internal/ingress/live_socket_test.go` (289) — PASS
- [x] A-090 `internal/runtime/p8_mode_control_test.go` (280) — PASS
- [x] A-091 `internal/replay/thermal_chamber_test.go` (191) — PASS

### Schema / SQL / proto / domain data

- [x] A-092 `docs/design/contracts/storage-schema-v1.sql` (675) — FINDINGS · HIGH → [A-092-design-storage-schema-v1.md](A-092-design-storage-schema-v1.md)
- [x] A-093 `migrations/001_initial.sql` (491) — PASS
- [x] A-094 `migrations/003_lifecycle_fencing.sql` (237) — PASS
- [x] A-095 `internal/spec/schema.json` (781) — FINDINGS · HIGH → [A-095-spec-schema.md](A-095-spec-schema.md)
- [x] A-096 `internal/eventschema/registry_data.json` (852) — PASS
- [x] A-097 `docs/design/contracts/runtime-v1.proto` (366, canonical proto source for the generated `proto/agenticstream/runtime/v1/` code) — PASS

## Summary

Audit complete 2026-09-11. 97 files audited (69 production, 22 test, 6 schema/SQL/proto/data), 2 generated files skipped.

**Verdicts: 20 PASS · 77 FINDINGS · 16 HIGH.**

### HIGH findings (fix before any further feature work)

| # | File | Finding |
|---|------|---------|
| 1 | `internal/episodes/executor.go` (A-003) | Success-path persist tx uses cancellable ctx (lost decisions, stuck `running`); epoch-kill quarantine UPDATEs rolled back by own error return → poisoned admission loop. Empirically proven. |
| 2 | `internal/actions/dispatcher.go` (A-002) | Expired-lease reclaim finalizes before tenant/intent populated → rollback every attempt → permanent head-of-line blocking of the action plane. |
| 3 | `internal/operators/operators.go` (A-005) | `on_close` windows (schema + runtime default) never emit a feature — no window-close path exists; flagship example silently worked around in tests. |
| 4 | `internal/clock/clock.go` (A-058) | `Virtual.Advance` skips due timers behind a not-yet-due head after partial sort — breaks timer-driven replay. Empirically proven. |
| 5 | `internal/policy/policy_evaluate.go` (A-041) | `requires_approval` shortcut bypasses R3/R4 denial and creates an unbounded approve→re-pending loop; intent can never dispatch. |
| 6 | `internal/decisions/validator.go` (A-013) | `parameters.target` bypasses entity binding when `entity_id` absent — model can steer an effect at an unverified target. |
| 7 | `internal/engine/engine_state.go` (A-030) | `lineageID` hashes evidence IDs with no separator → concatenation collisions silently mis-attribute evidence (`ON CONFLICT DO NOTHING`). |
| 8 | `internal/executor/native/openai.go` (A-038) | Nil client falls back to no-timeout `http.DefaultClient` (stalled provider hangs worker); streamed requests never set `include_usage` → zero-cost settlement. |
| 9 | `internal/executor/native/native.go` (A-012) | Timeout/cancel terminals settle `Usage{}` → under-counted `spent_micro` weakens the cost kill switch for the most expensive episodes. |
| 10 | `internal/ingress/jsonl.go` (A-049) | Quarantine ID `line:<N>` not trace-scoped → second malformed trace flips prior record to `rejected` and aborts ingestion. |
| 11 | `internal/ingress/simulator.go` (A-019) | Hardcoded `"mode"` channel branch in Go; no data entry — violates channels-are-data invariant. |
| 12 | `internal/replay/replay.go` (A-001) | ~450 LOC unreachable Mode/Shadow/Recorded machinery wired nowhere; admission logic diverges from production pipeline — golden-fidelity risk. |
| 13 | `docs/design/contracts/storage-schema-v1.sql` (A-092) | Contract is a pre/post-cutover chimera matching no achievable migration state; changed without the ADR its own header requires. |
| 14 | `internal/spec/schema.json` (A-095) | `executor.skills` implemented end-to-end but rejected by the schema (`additionalProperties:false`) — P5 skill path unreachable from YAML. Schema also admits window/operator kinds and aggregates the runtime rejects; 5 reducer strategies are silent no-ops. |
| 15 | `internal/storage/storage_test.go` (A-080) | `TestOpenIsIdempotent` mtime assertion can never fail — false confidence on data-wiping regressions. |
| 16 | `internal/engine/engine_test.go` (A-078) | Fixed 100 ms sleep gates the SQLite busy-retry test — CI-flaky false pass; goroutine leak on timeout path. |

### Recurring themes

1. **Test-only exported surface / dead machinery** — the largest structural debt. Unwired-but-maintained code: replay Mode machinery, quarantine release/redrive, engine partition loop (`run`/`runBatch`), `SerialEffector.SafeStop`, `Exchange`, `NewUDSTransport`, `Ledger.Recover`, `DeterministicBaseline`, zero-caller accessors in `lifecycle.go`/`serial_session_state.go`/`live_socket.go`/`uds.go`. Either wire, delete, or demote to internal test helpers.
2. **Silent no-op spec surface** — schema-accepted fields the runtime ignores (`where`, `reopenCooldown`, `watermarkStrategy`, `count`, `halfLife`, reducer `limit`, 5 reducer strategies). Digests include them, behavior doesn't. Fail-closed at compile or remove from schema.
3. **Divergent duplicates** — feature-save paths (engine_timers vs engine_apply), bind-state machines (serial_session_state vs serial_session), `_event_time` key convention (engine_restore vs situations), budget wall-time parsing (worker_executor ×2), hand-duplicated file sets in runartifact, hand-duplicated type lists in notifycontract.
4. **Swallowed errors that gate correctness** — manifest enrichment (A-032), `DigestForVersion` (A-050), interlock errors → permanent denial (A-028), migration-table probe (A-046).
5. **Test hygiene** — 50+ line copy-pasted setup blocks across cognition/episodes/actions test files; assertions that cannot fail; coverage gaps exactly on failure paths (lease steal, epoch kill, `set_union` reducer, slope values).

### Recommended order

1. Fix the 10 code HIGHs (1–11 above) — each has a concrete fix in its audit file.
2. Decide wire-or-delete for every test-only exported symbol (theme 1) in one sweep.
3. Reconcile schema/spec contract (A-092, A-095): regenerate storage contract from migrations, add `skills` to spec schema or remove the feature, cut silent no-ops.
4. De-duplicate divergent pairs (theme 3).
5. Repair the false-confidence tests (A-072, A-078, A-080) and extract shared test setup helpers.

Each FINDINGS file lists its own acceptance bar; close a finding only when every bar line is true.
