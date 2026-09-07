# Build Plan: Agentic Stream — Right Half + Integration Readiness

Status: **revision 2 (2026-08-12)** — re-audited against current source. The repo advanced
enormously since revision 1; most of the phases below are now **built**. This revision adds a
status map (§A) and a re-scoped **remaining critical path for B-full** (§B) at the top. The
detailed phases (§0–§15) are retained as reference and as the spec for the deferred items.
Target architecture: **B-full** — Tamoz is the streaming gRPC episode worker; the stream
*supervises* episodes (budgets, cancellation, replay ledger). Stream-side stream support is the
main offering, so the governance items are in scope — just sequenced after the first loop.

Repo: `~/my-projects/agentic-stream` @ `main`, HEAD `bed8576` ("add runtime telemetry and
operations readiness"). Go 1.26, `modernc.org/sqlite`, `cel-go`, `cobra`, **`grpc-go` now present**; the
proto is compiled at `proto/agenticstream/runtime/v1/*.pb.go`. 23 internal packages, 12
migrations.

---

## §A. Status map (verified against source, revision 2)

**Legend:** ✅ built · 🟡 partial · ⬜ not started.

| Phase | Item | Status | Evidence |
|---|---|---|---|
| P0.1 | RFC 8785 canonicalization + domain digest | ✅ | `harden canonical contracts`; `internal/contractsv1`, `internal/canonicaljson` |
| P0.2 | Versioned schemas + CloudEvents envelope | ✅ | `versioned contract schemas and cloud events` |
| P1.1 | Proto reconcile & **freeze** (fence identity, attempt terminals) | ✅ | `freeze fenced worker protocol v1` |
| P1.2 | gRPC toolchain + codegen | ✅ | `proto/agenticstream/runtime/v1/runtime-v1{,_grpc}.pb.go` |
| P1.3 | gRPC worker server + evidence-tools host + capability tokens | ✅ | `internal/worker/server.go`, `internal/evidence/server.go`, `scoped evidence tools over private uds`, `per-dispatch capability composition seam`, migration `010_evidence_call_ledger` |
| P1.4 | Native executor + **model provider** | ✅ | `internal/executor/native`, deterministic provider, OpenAI-compatible streaming adapter, bounded read-tool loop |
| P1.4 | **Executor conformance harness** (what Go workers code against) | ✅ | `internal/executor/conformance`; fake, in-process streamed, and separate-process Go worker fixtures pass |
| P2 | Lifecycle separation + `(episode_id, attempt_id, fence)` | ✅ | `episode lifecycle fencing`, `owner-aware episode attempt fencing`, migrations `003`,`011`,`012` |
| P3.1 | Decision validator (replaces auto-accept) | ✅ | `internal/decisions/validator.go`, `validate and persist typed decisions` |
| P3.2 | Prompt/objective content addressing | ✅ | durable prompt/objective digests in episode requests |
| P3.3 | Event schema registry | ✅ | `internal/eventschema`, deployment registration, append-time validation |
| P4.1 | Stream capability host + dependency-direction test | ✅ | `internal/policy/capabilityhost_test.go` enforces forbidden imports and runs the injection corpus |
| P4.7 | Source-health gating | ✅ | heartbeat loss emits `uncertain`; dependent Situation completeness changes publish immutable versions and policy gates R2–R4 |
| P4.2 | Policy gateway + idempotent outbox | ✅ | `enforce policy and idempotent action outbox`, migration `004_policy_audit` |
| P4.3 | Principal registry / separation of duty | ✅ | durable principal/authority checks in `internal/policy` |
| P4.4 | Watch-condition intent + effector | ✅ | restricted CEL watch effector with owner/interlock fencing |
| P4.5 | Compensating intents | ✅ | compensating intent binding and policy validation |
| P4.6 | Interlock readiness | ✅ | durable interlock reader and write fences |
| P4.8 | Outbox + **concrete effector** | ✅ | simulated effector, watch effector, dispatcher and reconciliation |
| P4.9 | Quarantine + gap records / aggregate cost ceiling + kill switch | ✅ | ingress quarantine/redrive, event gaps, cost controller and kill switch |
| P5 | Reconsideration admission (dedup) | ✅ | `admit deduplicated reconsideration episodes`, migration `005` |
| P6.1 | Durable notification log + cursor (Channel B) | ✅ | `durable cursor notifications`, migration `006` |
| P6.2 | SSE application binding | ✅ | `/v1/events`, cursor resume, auth, bounded lag, audited expiry and poison retry |
| P6.3 | Channel B lifecycle event types | ✅ | transactional `io.agenticstream.*.v1` approval, command, and outcome events with durable cursors |
| P6.4 | Approval workflow + withdrawal + assertion binding | 🟡 | approval assertions and withdrawal paths exist; key-rotation rehearsal remains |
| P7 | Replay isolation + worker-aware modes | ✅ | `isolated worker-aware replay modes`, `harden replay isolation` |
| P8.1 | Trace propagation | ✅ | durable W3C context, OpenTelemetry spans, OTLP/HTTP export, and asynchronous links |
| P8.2 | HTTP surface | ✅ | RFC 9457 readiness, loopback binding, authenticated SSE, remote-worker mTLS flags |
| P8.3 | Telemetry and operational release | ✅/🟡 | bounded metrics, OpenTelemetry spans/export, runbooks and security checklist; environment evidence is postponed |
| — | Runtime ownership lease + crash recovery (beyond original plan) | ✅ | `009`,`011`,`012`, `internal/runtime` — robustness the plan didn't scope |
| — | **Live pipeline composition** | ✅ | `run-live` handles bounded batches; `serve --spec ... --trace ...` owns replay polling and `serve --spec ... --live-socket ...` owns reconnecting live normalized JSONL ingestion with runtime cancellation and fatal-error propagation |

**Reading:** the deterministic engine, the fenced gRPC worker protocol, decision validation,
policy+outbox, reconsideration, notifications, replay isolation, and trace are **done**. What is
missing splits into two buckets: **(1) the essential glue that makes an episode actually run
end-to-end**, and **(2) governance/safety features** (principals, watch conditions, compensating
intents, interlock, quarantine, cost ceiling, schema registry, prompt addressing).

---

## §B. Remaining critical path for a first supervised B-full loop

The bounded first loop is now implemented as `run-live`: simulator or normalized JSONL → stream
→ native Go executor or current-v1 Go worker → Decision → policy → simulated effect. The remaining
items below are production-hardening or deployment-level gates, not missing deterministic core
behavior.

### B.1 — Essential (blocks the loop)

| # | Work | Where | Why essential |
|---|---|---|---|
| **S1** | Continuous ingestion and scheduling under `serve` | `cmd/agentic-stream`, `internal/runtime`, `internal/ingress` | ✅ `serve --spec ... --trace ...` polls replay JSONL; `serve --spec ... --live-socket ...` consumes reconnecting live normalized JSONL; both run the complete pipeline under the runtime owner. |
| **S2** | Separate-process Go worker conformance fixture | `internal/executor/conformance`, `internal/worker` | ✅ Test binary launches a Go worker over a real private Unix socket and runs the shared conformance suite. |
| **S3** | OpenTelemetry spans/exporter and asynchronous span links | `internal/telemetry`, worker boundary | ✅ Runtime and worker spans export via OTLP/HTTP and link to durable W3C source contexts. |
| **S4** | Environment release evidence | `docs/runbooks/runtime-operations.md` | ⏸ Postponed by scope decision; future production-release gate. |

### B.2 — Deferred to the hardening pass (part of the offering, not the first loop)

Principal registry + separation of duty (P4.3) · approval assertion binding + withdrawal (P6.4)
· watch conditions (P4.4) · compensating intents (P4.5) · interlock readiness (P4.6) ·
quarantine + gap records and aggregate cost ceiling + kill switch (P4.9) · event schema registry
(P3.3) · prompt content-addressing (P3.2) · RFC 9457 + mTLS hardening (P8.2). Each has its full
spec below. The capability-host dependency test, source-health propagation, and Channel B lifecycle
events are now implemented; only environment verification and the postponed release evidence remain
outside the current pass.

> **Net:** the deterministic and bounded supervised loop and continuous JSONL serving loops are
> complete. OpenTelemetry instrumentation and export are implemented.
> Environment release evidence is explicitly postponed. No Python worker or
> legacy compatibility layer is required.

---

## Reference: the original phased plan (§0–§15)

The sections below are the detailed spec. Items marked ✅ in §A are done — read them for
context; build the ⬜/🟡 items. The sequencing in §13 is superseded by §B for B-full.

Each phase lists concrete files, schema migrations, acceptance tests, and dependencies.
The **§** references point to `integration/STREAM_RESPONSIBILITIES.md` unless noted; **M/Gate**
references point to `agentic-stream/design/IMPLEMENTATION_PLAN.md`.

---

## 0. What exists today vs. what this plan builds (verified in source)

**Built and working (the deterministic left half, ≈ M0–M2):**
`internal/ingress` (JSONL), `internal/eventlog` (append-only + dedup, carries `traceparent`),
`internal/storage` (SQLite WAL + migration runner), `internal/spec` (compiler + restricted CEL
+ JSON-schema validation), `internal/operators` (aggregate/slope/missing-heartbeat),
`internal/situations` + `internal/engine` (reducer, partition worker, checkpoint),
`internal/cognition` (`engine.go` 430L + `scheduler.go` 318L — trigger scoring, debounce,
coalesce, supersede; the most mature integration-relevant code), `internal/episodes/assembler.go`
(builds the Episode Request + snapshot digest, `INSERT INTO episodes … status='queued'`),
`internal/replay/replay.go` (**deterministic mode only**).

**Present in schema but zero Go writes them** (`migrations/001_initial.sql`): `intents`,
`approvals`, `commands`, `outbox`, `outcomes`, `replay_jobs`, `timers`, `artifacts`. The whole
policy/action/outcome plane is unimplemented.

**The concrete defects this plan fixes:**

| Where | Defect | Fixed in |
|---|---|---|
| `internal/canonicaljson/canonicaljson.go` | Self-described "subset of RFC 8785": `encodeFloat` uses `FormatFloat('f',-1)` (emits `0.0000001`/`1e21`-region, not JCS `1e-7`/`1e+21`); keys sorted by `sort.Strings` (UTF-8 bytes, not UTF-16); `Digest()` (`:38-45`) is bare `SHA256(jcs)` with **no domain separation and no `sha256:` prefix**; no reject rules (`NaN`/`Inf` only). | P0.1 |
| `internal/episodes/executor.go:99-112` | `Runner` inserts every Decision `validation_status='accepted'` with `validation_json='{}'`. **No decision validation, no policy, no digest over canonical bytes** (`sha256.Sum256(outcome.DecisionJSON)` hashes raw bytes). | P3.1 |
| `internal/episodes/executor.go` | `Executor` is an **in-process Go interface**; `FakeExecutor` (`fake_executor.go`) is used only from `runner_test.go`; the CLI has no `serve` and nothing listens on a socket. | P1.x |
| `docs/design/contracts/runtime-v1.proto` | Uncompiled; handshake lacks `non_interactive`/`worker_id`/`contract_version`/`shadow_capable`/`supports_kinds`/`emits_complete_replay_ledger`; `TerminalStatus` uses business outcomes (`DECIDED/NO_ACTION/NEEDS_HUMAN`) not attempt-scoped (`PRODUCED/DECLINED/…`); no `kind`, `lane`, `risk_ceiling`, `intent_types`, no `(episode_id, attempt_id, fence)`, no RECONSIDER extras. | P1.1 |
| `migrations/001_initial.sql:305-321` | `episodes.status` is a single 13-value column conflating four state machines; no `episode_attempts`, no `verifications`, no `current_fence`/`current_attempt_id`. | P2.1 |

---

## 1. Strategy, sequencing, and gates

**Ordering principle:** contract → transport → lifecycle → validation → policy/action →
reconsideration → notifications → replay → trace/ops. You cannot build "governed action" without
fencing, and you cannot test fencing without the worker port, so the early phases are strictly
ordered; later phases parallelize.

**Map to the design's release gates:**

| Gate | Meaning | Closed by |
|---|---|---|
| **A** — deterministic foundation (hardened) | canonical hashes byte-identical across runs *and across Go/Ruby* | P0 |
| **B** — bounded cognition | no event invokes a model directly; budgets enforced; superseded episode can't produce an accepted Decision | P1–P3 |
| **C** — safe effects | model has no effector handle; stale intent can't become a command; crash at outbox boundary → no dup; unknown outcome → reconciliation; replay can't load effectors | P4, P5, P7 |
| **D** — operational release | mTLS/socket/secrets/poisoning/token/retention security review; 24h soak; runbooks | P6, P8 |

**Effort framing (one senior engineer, from the design's own estimates):** P0 ≈ 1 wk; P1 ≈ 2–3 wk
(new gRPC surface); P2 ≈ 1.5 wk; P3 ≈ 1.5 wk; P4 ≈ 3–4 wk (the largest); P5 ≈ 1 wk; P6 ≈ 2–3 wk;
P7 ≈ 1.5 wk; P8 ≈ 2 wk. Roughly **16–20 engineer-weeks** to a Tamoz-integrable, Gate-C release,
which matches the design's ~15-week M0–M4 estimate plus the integration deltas.

**Program prerequisite (decide before P0):** the shared contract package
(`CONTRACTS.md §1`) does not exist. Vendor `integration/contracts/canonicalization-vectors.json`
and the `schemas/v1/*` into the repo under `internal/contractsv1/testdata/` (or a `contracts/`
submodule), pinned by a `contract_version` constant. Both Go and Ruby read the **same** vectors
file. This gates P0.1.

---

## 2. Phase P0 — Contract foundation (blocking; hardens Gate A)

### P0.1 · RFC 8785 canonicalization + domain-separated digest — `§3.1`
**Rewrite `internal/canonicaljson/canonicaljson.go`:**
- **Numbers:** replace `encodeFloat` with true ECMAScript `Number::toString` (shortest
  round-trip with JCS exponent rules: plain notation for `1e-6 … 1e21`, exponential outside).
  Go's `strconv.FormatFloat(f, 'g', -1, 64)` is *not* JCS; use a Ryū-based shortest formatter and
  apply the ES6 exponent-threshold rules, or vendor a JCS lib (`gowebpki/jcs`). Reject `NaN`,
  `±Infinity`, `-0`, and integers outside ±(2^53−1).
- **Keys:** sort by **UTF-16 code unit**, not `sort.Strings`. (Surrogate-pair ordering: U+1F600
  sorts before U+FB2C.)
- **Strings:** minimal escaping per the JCS fixed escape table.
- **Reject duplicate object keys** at decode (the current `json.Unmarshal` silently keeps last).
- **Digest:** new signature `Digest(domain Domain, v any) (string, error)` returning
  `"sha256:" + hex(SHA256(domainBytes ‖ jcsBytes))` where `domainBytes =
  "situation-runtime/<type>/v<major>\n"`. Add a `Domain` enum (snapshot/spec/decision/intent/
  command/event/envelope/test) and a **constant-time** `Verify(domain, v, digest)` helper.

**Update all 4 callers** to pass a domain and consume the `sha256:` form:
`internal/cognition/engine.go:218`, `internal/episodes/assembler.go:111` & `:131`,
`internal/spec/compiler.go:161` & `:165`. Also fix `internal/episodes/executor.go:103` (currently
`sha256.Sum256(outcome.DecisionJSON)` on raw bytes) to use the canonical Decision digest.

**New test `internal/canonicaljson/vectors_test.go`:** read the vendored
`canonicalization-vectors.json`; reproduce all **16 accept** vectors (canonical string + digest),
**construct both `native_only`** double-vs-integer cases in Go, and **refuse all 8 `reject`**
inputs. Add a fixture proving a Go digest equals the Ruby digest for `snapshot-v1`.

*Accepts when:* the Go suite reproduces every vector; a float-bearing snapshot digest is identical
to Tamoz's (joint invariant #1). **This is the load-bearing blocker — do it first.**

### P0.2 · Shared schema + CloudEvents envelope types — `CONTRACTS.md §4, §7`
- Vendor JSON Schema 2020-12 for snapshot/decision/intent/command/outcome under
  `internal/contractsv1/schemas/v1/` with `urn:situation-runtime:schema:<type>:v1` `$id`s; validate
  on ingress and egress.
- Add a `CloudEvent` type + the `envelopedigest` canonical projection (`specversion,type,source,id,
  subject,time,dataschema,tenantid,partitionkey,classification, digest(data)`) under domain
  `situation-runtime/envelope/v1\n`. Needed by P6; define now so Channel B is a binding, not a
  redesign.
- Add a `contract_version` constant and surface it in `cmd/agentic-stream version`.

*Accepts when:* a spec referencing an undeclared payload field or a bad unit fails validation; the
`version` command prints contract + protocol versions.

---

## 3. Phase P1 — Channel A: the worker port (M2.4/M2.5 + `§3.16`)

### P1.1 · Reconcile & freeze `runtime-v1.proto` — **includes the fence wire-contract**
Bring the proto to PROTOCOL.md §2/§3.3 and LIFECYCLES §7 **before** compiling, because Tamoz codes
to exactly this shape:
- `HandshakeRequest/Response`: add `worker_id`, `contract_version`, `non_interactive` (bool, must
  be true), `shadow_capable`, `counterfactual_capable`, `emits_complete_replay_ledger`,
  `repeated Kind supports_kinds`.
- `EpisodeRequest`: add `Kind kind` (`DIAGNOSE`/`RECONSIDER`), `Lane lane` (`fast`/`deep`/`batch`),
  `RiskClass risk_ceiling`, `repeated string allowed_intent_types`, evidence `time_range`, and a
  distinct `cancellation_key` vs `supersession_key`. Add a `Reconsideration` sub-message (prior
  Decision, executed commands, their Outcomes, the correction) populated only for `RECONSIDER`.
- **Identity on every `EpisodeEvent` and `DecisionProposed`:** `episode_id`, `attempt_id`, `fence`.
- **`TerminalStatus`** → attempt-scoped: `PRODUCED`, `DECLINED`, `CANCELLED`, `FAILED`, `TIMED_OUT`
  (delete `DECIDED/NO_ACTION/NEEDS_HUMAN/SUPERSEDED/BUDGET_EXHAUSTED`; those are stream judgments in
  machines 2–4, not worker terminals).
- Keep the `EvidenceTools` reverse service; add the `capability_token` requirement to its calls.

### P1.2 · gRPC toolchain + codegen
Add `google.golang.org/grpc` (direct), `protoc-gen-go`, `protoc-gen-go-grpc` pinned in `tools.go`;
add a `Makefile generate` target; generate into `internal/runtimev1/`. CI regenerates and checks
for drift.

### P1.3 · gRPC episode server + capability tokens + evidence-tools host
- `internal/worker/server.go`: UDS gRPC `EpisodeWorker` server (mTLS transport for any remote
  worker — PROTOCOL §1). Handshake **refuses**: `non_interactive:false`, unknown major
  `contract_version`, or `emits_complete_replay_ledger:false` for a spec that requires recorded
  replay (`§3.16`).
- `internal/worker/token.go`: per-episode **JWT** (`CONTRACTS §11`) — `iss/aud/sub/exp/nbf/jti/
  tenant/entity/time_range/tools/intent_types/risk_ceiling/snapshot_digest`, signed with a key the
  worker verifies but cannot mint. Single-use `jti`.
- `internal/worker/evidence.go`: the `EvidenceTools` reverse service, scoped strictly to the
  token's `tools`/`tenant`/`entity`/`time_range`; oversized results spill to `artifacts` and return
  an `ArtifactRef`.

### P1.4 · Dispatch path + native executor + conformance harness (M2.4/M2.5 exit)
- Replace the in-process-only `Runner.RunOnce` path: dispatch a queued episode over the port,
  assigning `attempt_id` + `fence` at dispatch (schema arrives in P2, so gate P1.4's fence behavior
  on P2 or stub `fence=1` until then — note the dependency).
- `internal/executor/native/`: the native episode executor from M2.4 — direct model-provider port,
  deterministic fake provider, one OpenAI-compatible streaming adapter, read-tool loop, argument
  validation, cancellation, **all hard budgets**, typed terminal outcomes, model/tool ledger, one
  structured-output repair. *Do not* build shell/HTTP tools, memory, or subagents (that is Tamoz).
- **Executor conformance suite** (`internal/executor/conformance/`): a runnable harness the fake,
  the native, and an external worker all pass. **This is the artifact Tamoz codes its worker
  against** — promote `FakeExecutor` out of `runner_test.go` into it.
- Non-interactive rule: an `interrupt` inside an episode is a typed terminal `FAILED` with
  `interrupt_in_non_interactive_episode`, never a wall-clock wait (`§3.16`, PROTOCOL §3.4).

*Accepts when:* a worker declaring `non_interactive:false` is rejected at handshake; an interrupting
worker fails fast (not a timeout); fake + native executors pass the same conformance suite; a
predictive-maintenance warning creates one bounded Decision (Gate B).

---

## 4. Phase P2 — Lifecycle separation & fencing (`§3.18`, LIFECYCLES)

### P2.1 · Migration `003_lifecycle_fencing.sql`
Additive, never editing shipped tables' meaning:
- `episodes`: rename `status` → `lifecycle_status` restricted to the 7 aggregate values
  (`admitted/running/concluded/closed/superseded/expired/abandoned`); add `current_attempt_id`,
  `current_fence`. Replace `one_live_episode_per_situation` predicate with
  `lifecycle_status IN ('admitted','running')`.
- **new `episode_attempts`**: `attempt_id` PK, `episode_id`, `fence`, `status` (the six attempt
  terminals), `started_at`, `ended_at`, `terminal_json`, `artifact_manifest_json`; unique
  `(episode_id, fence)`.
- `decisions`: add `attempt_id`, and split `validation_status` into the machine-2 values
  (`proposed/accepted/rejected`) + `rejection_reason` (the 11 durable reasons in LIFECYCLES §4).
- **new `verifications`**: `intent_id`, `command_id`, `outcome_id`, `verdict`
  (`verified/refuted/inconclusive/superseded_before_verification`), `reconciled_at`; unique on
  `intent_id`.
- The removed business-outcome values (`decided/no_action/needs_human`) become a **view** derived
  from machines 2–4.

### P2.2 · The four state machines + fence assignment
`internal/episodes/lifecycle.go`: implement Episode aggregate, Worker-attempt, Decision-validation,
Outcome-verification as separate machines. Assign `fence` (monotonic per episode from 1) and a
fresh `attempt_id` at each dispatch; record `current_*`. Reject inbound worker work by identity:
`unknown_episode` / `stale_attempt` (fence < current) / `wrong_attempt` / `terminal_attempt` /
`episode_closed`, **independent of snapshot equality**. All rejections durable with reason.

### P2.3 · Cancellation → abandoned → re-dispatch
Mark attempt `cancelling`, cancel the RPC; on ack → `cancelled`; on grace-period expiry →
`abandoned` and dispatch a new attempt at `fence+1`. Late output from the abandoned attempt is
refused `stale_attempt`.

*Accepts when:* a retry against the **identical** snapshot has the prior attempt's Decision refused
as `stale_attempt`; an episode simultaneously records a successful attempt and a rejected Decision;
one Decision's two intents end `succeeded` and `manual_review` independently.

---

## 5. Phase P3 — Decision validation + provenance addressing

### P3.1 · Decision validator (M3.1) — replaces the auto-accept
`internal/policy/decision_validator.go`: schema validation; identity/snapshot/version checks;
evidence visibility + existence; facts/inferences separation; confidence + expiry; allowed intent
types; `risk_ceiling` ceiling; normalized Decision hash (via P0.1 domain digest);
accepted/rejected persistence with the machine-2 `rejection_reason`. **Delete the hard-coded
`'accepted'` insert** in `executor.go:107`. One bounded repair attempt if configured, else fail
closed.

*Accepts when:* forged evidence, stale snapshot, unsupported intent, oversized free text, and a
still-failing repair all reject fail-closed; a duplicate Decision delivery is idempotent.

### P3.2 · Prompt/objective content addressing — `§3.12`
Content-address prompts and objectives (`episodes.prompt_version` is free text today, copied
verbatim in `assembler.go`); store the digest in episode provenance so recorded replay cannot
reproduce a changed prompt as "identical."

*Accepts when:* changing prompt text without changing its label yields a different provenance digest.

### P3.3 · Event schema registry — `§3.11`
Add a registered `event_schemas` table; reference it from `SituationSpec.input`; build the typed CEL
environment from it. (`§7.1` requires rejecting unknown payload fields; today `input` has no schema
pointer.)

*Accepts when:* a spec referencing an undeclared payload field fails compilation with a diagnostic
location; a unit mismatch fails at compile, not runtime.

---

## 6. Phase P4 — Policy & governed action plane (M3.2/M3.4 + `§3.2/3.5/3.6/3.13/3.14/3.15/3.19`)

The largest phase. Order inside it: capability host → policy chain → principals → intents/effectors
→ outbox/reconciliation → interlock/source-health → quarantine/cost-ceiling.

### P4.1 · Stream capability host — `§3.19`, THREAT_MODEL §4
`internal/policy/capabilityhost.go`: build the episode-facing tool surface from an **allowlist that
never holds a reference** to a file-mutation, shell, MCP, elicitation, or effect-journal module.
Prove with a **dependency-direction test** and an injection corpus. This is what makes invariant #5
structural rather than prose.

### P4.2 · Policy gateway — ordered chain (M3.2)
`internal/policy/gateway.go`, the 8-step chain: Decision accepted → intent schema/capability →
Situation freshness → preconditions → risk class → quota/rate → approval → Command/outbox. Initial
policies: R0 auto, R1 auto+rate-limit, R2 approval, R3/R4 deny, replay always simulate/deny.
Record `policy_version`.

### P4.3 · Principal registry + separation of duty — `§3.14`
`migrations/004`: `principals`, `roles`, per-`(tenant, entity, risk_class)` approval authority.
Replace the single API bearer token. Enforce **relay ≠ approver**; an approval asserted for an
unauthorized principal is denied with an audit record.

### P4.4 · Watch-condition intent + internal effector — `§3.5`
Add `install_watch_condition` as a first-class R0 intent with an internal effector that installs a
**scoped, expiring, count-bounded derived trigger**; the expression compiles under the existing
restricted CEL. **Structural bound:** the effector holds no reference to the spec store or the
deployment path (dependency-direction test) — it cannot alter a SituationSpec.

### P4.5 · Compensating intents — `§3.6`
Support `compensates: <command_id>` on any intent type; route through the full policy chain under
**its own** risk class. A compensating intent that is R3 is denied by default — never auto-safe.

### P4.6 · Interlock readiness — `§3.2`
`internal/policy/interlock.go`: a fail-closed read-only interlock read at the **narrowest point**
(immediately before command delivery, and re-read/asserted by the effector at accept). Port Tamoz's
`InterlockReader` (read-only surface, dependency-direction test proving production code cannot
reference a mutable harness).

### P4.7 · Source-health gating — `§3.15`
Declare a dependency from a Situation type to the source-health Situations of its inputs, feeding
completeness and precondition evaluation (a door sensor going quiet degrades the refrigeration
Situation and fails an R2 intent closed).

### P4.8 · Outbox, effectors, reconciliation (M3.4)
`internal/action/`: Intent + Command stores (now written), atomic outbox creation, dispatcher with
leases + idempotency, result/outcome events, the reconciliation state machine, a `simulated`
effector and a `maintenance.ticket` SQLite demo effector. Crash-safe at every dispatch boundary.

### P4.9 · Quarantine/gap records + cost ceiling — `§3.3`, `§3.13`
- Quarantine table + bounded log + gap record on overflow + inspection/re-drive path (port Tamoz's
  semantics). `§21` routes every invalid event here today with nowhere to land.
- Tenant + global **cost ceiling with a durable kill switch** — under `correct_and_reconsider`,
  episode count scales with *data quality*, so one flaky gateway across 120 zones is a real exposure
  with an expensive external worker.

*Accepts when (Gate C):* a model has no effector handle; a stale intent cannot become a command; a
crash at any outbox boundary produces no duplicate accepted effect; an unknown outcome enters
reconciliation, not re-execution; an installed watch condition fires once, expires on schedule, and
cannot alter a spec; an R3 compensating intent is denied by default; a command dispatched after an
interlock trips is refused; a synthetic correction storm halts at the cost ceiling and preserves
ingress; the injection corpus reaches no filesystem or effector call.

---

## 7. Phase P5 — Reconsideration admission (`§3.4`)

`internal/cognition/reconsideration.go`: detect deterministically that *a correction superseded a
Situation version whose accepted Decision produced an already-executed command*; create a trigger
evaluation with reason `prior_action_invalidated`; admit an episode `kind: RECONSIDER` carrying the
prior Decision, executed commands, their Outcomes, and the correction. **Dedup key
`(situation_id, superseded_version, invalidated_command_id)`.**

*Accepts when:* the freezer golden trace produces **exactly one** reconsideration admission;
replaying the trace twice yields one; a second unrelated correction yields a second.

---

## 8. Phase P6 — Channel B notifications + durability + approvals (`§3.7/3.8`, PROTOCOL §9)

### P6.1 · Durable notification log
`migrations/005`: a `notifications` table with a **monotonic per-`(tenant)` cursor**, each row
written **in the same transaction** as the state change that caused it (a notification implies the
change committed). Retention ≥ 7 days and never shorter than the longest reconciliation window;
enforced by a durable job with a dry run.

### P6.2 · SSE application binding (`CONTRACTS §4.6`, PROTOCOL §9.2)
`internal/notify/sse.go`: `id:` = cursor (**not** the CloudEvents `id`), `event:` = CE `type`,
`data:` = one CE JSON frame, `retry:` = reconnect hint; idle comment frames; **poison-event skip**
after bounded retry (`subscriber_skipped`); **backpressure disconnect** (`subscriber_too_slow`,
resume from last ack); per-subscriber credential + event-type allowlist; at-least-once with
dedup on `source`+CE `id`. `cursor_expired` → refuse resume, force an **audited resnapshot** (§9.1).

### P6.3 · Event types
Emit `situation.version.published`, `situation.superseded`, `approval.requested/withdrawn/resolved`,
`command.dispatched`, `outcome.recorded/reconciled`, `reconsideration.admitted` (types per
PROTOCOL §6.1), each a CloudEvent with the P0.2 `envelopedigest`, **signed** where it crosses a
trust boundary.

### P6.4 · Approval workflow + withdrawal + assertion verification (M3.3, `§3.7`, PROTOCOL §5/§10)
Approval request/expiry; **revalidate freshness, non-supersession, completeness, source health,
quorum, preconditions, interlock at policy time**, and **re-read the interlock at dispatch**.
Verify the relayed approval **assertion** (PROTOCOL §10): signature over the canonical form under
`situation-runtime/approval-assertion/v1\n`, the 11 bound fields, single-use `nonce` recorded
durably, `relay_id ≠ approver_id`, key rotation with overlap window. Emit `approval.withdrawn`
**before** a human can act on a superseded request; a late submission is refused
`situation_version_conflict` (RFC 9457).

*Accepts when:* a worker disconnected for a command's whole lifecycle receives every outcome on
reconnect via `Last-Event-ID`; `cursor_expired` forces an audited resnapshot; a poison event is
skipped after bounded retry; a slow subscriber is disconnected rather than buffered unboundedly;
supersession during a pending approval emits a withdrawal and a late approval is refused.

---

## 9. Phase P7 — Replay across the boundary + credential isolation (`§3.9/3.10`, M3.5)

### P7.1 · Structural credential isolation — `§3.9`
Make the replay runtime **constructor accept no credential argument at all**; prove with a
**poison-resolver** test that panics on any resolution and is never invoked by any of the four modes.

### P7.2 · Worker-aware replay modes — `§3.10`, amend ADR-009
- `deterministic` — operators/Situations only (already built).
- `recorded` — **the stream replays from its own model/tool ledger; the worker is never called.**
  This resolves the contradiction that a Tamoz worker's SQLite/memory/skills can't be reproduced.
- `shadow` — call the worker with effects disabled; **pin and report** the per-episode artifact
  manifest (prompt/skill/tool/model/contract/memory digests); a diff because memory grew is a
  legitimate reported finding, not a silent absorption.
- `counterfactual` — commands route to a **simulator** effector (this is where `streams-simulator`
  plugs in as the plant).

*Accepts when:* the poison resolver is never invoked; a recorded replay of a Tamoz-executed episode
reproduces the accepted Decision **without** the worker running; a shadow comparison reports
memory/skill digest differences as findings.

---

## 10. Phase P8 — Trace propagation + ops hardening (`§3.17`, M4, Gate D)

### P8.1 · W3C Trace Context — `§3.17`, CONTRACTS §10
Propagate `traceparent`/`tracestate` into `EpisodeRequest` (gRPC metadata) and out on **every**
Channel B event; use span **links** for asynchronous stages (a Situation version caused by many
events; an Outcome arriving days after its Command). One trace spans source event → Situation
version → Decision → Command → Outcome across both products (joint invariant #11).

### P8.2 · HTTP surface + errors + transport auth (M4.2)
RFC 9457 Problem Details for the whole HTTP surface (Channel C); mTLS for remote workers; the
subscriber credential distinct from the worker capability token. MQTT 5 connector (M4.1) is
optional for the first pilot.

### P8.3 · Telemetry, runbooks, soak, security review (Gate D)
Metrics/tracing/logs; upgrade/backup/restore/disk-full/unclean-shutdown runbooks; 24-hour soak
showing bounded memory/queue/timer/DB growth; security review covering API binding, worker socket,
secrets, event poisoning, capability tokens, and **artifact retention** (the shadow-comparison
inputs the worker's manifest names).

---

## 11. Acceptance-test corrections to fold in (`§3.20`)

Five acceptance tests are known too weak; build the strengthened forms directly:

| Test | Strengthened form |
|---|---|
| P0.1 contracts (3.1) | Must also construct both `native_only` cases and refuse all 8 `reject` inputs — not just read the `accept` array. |
| P5 reconsideration (3.4) | Dedup key `(situation_id, superseded_version, invalidated_command_id)`; replay twice → one admission; a second unrelated correction → a second. |
| P4.4 watch condition (3.5) | **Structural**: the effector holds no reference to the spec store or deployment path (dependency direction), not "observe that no spec changed." |
| P6 outcome stream (3.8) | Must cover `cursor_expired` → audited resnapshot, poison-event skip after bounded retry, and slow-subscriber disconnect — not just a reconnect inside retention. |
| P8.1 trace (3.17) | Must assert the trace survives the gRPC boundary and **rejoins via span links** across the days-long gap to reconciliation — not just in-process. |

---

## 12. What to port from Tamoz rather than re-derive (`§4`)

Implemented, tested, and better than the current design there:

| Take | Into |
|---|---|
| `InterlockReader` (read-only surface + dependency-direction test) | P4.6 |
| `ActionBoundary` (revalidation at the narrowest point, TOCTOU closure) | P4.6/P6.4 |
| Quarantine semantics (typed durable rejection, hash-conflict discrimination, bounded log + gap records) | P4.9 |
| `CognitionAdmission` (pure, side-effect-free evaluator) | reference oracle for the Go scheduler conformance tests |
| Replay credential isolation (structural, not documented) | P7.1 |

---

## 13. Suggested first PRs (start here, in order)

1. **PR-1 · `canonicaljson` → RFC 8785 + domain digest + vectors test** (P0.1). Self-contained,
   load-bearing, unblocks every cross-language digest. Merge behind the vendored vectors file.
2. **PR-2 · vendor shared schemas + CloudEvents envelope + `contract_version`** (P0.2).
3. **PR-3 · proto reconcile & freeze** (P1.1) — no code yet, just the frozen contract Tamoz can
   read; circulate for joint sign-off.
4. **PR-4 · gRPC toolchain + codegen + empty server skeleton** (P1.2–P1.3 scaffolding).
5. **PR-5 · lifecycle+fencing migration and state machines** (P2.1–P2.2) — do this early; every
   later phase writes attempt/decision/verification rows.
6. **PR-6 · capability-token issuance + evidence-tools host** (P1.3).
7. **PR-7 · native executor + conformance harness** (P1.4) — the artifact Tamoz codes against.
8. **PR-8 · Decision validator replacing the auto-accept** (P3.1).

After PR-8 the stream can host an external worker, validate its Decisions, and fence its attempts —
the minimum for a first real Tamoz DIAGNOSE episode. P4 onward makes effects safe and governed.

---

## 14. Owner decisions required

1. **Executor architecture:** the proto implies gRPC-native workers; the code has an in-process
   `Executor` interface. Decide whether the native executor stays in-process behind an adapter or
   becomes a gRPC client of its own port. This gates ~half of P1/P4 structure.
2. **Contract package home:** vendored-and-pinned (recommended) vs. a shared repo/submodule (§1).
3. **Cost-ceiling defaults** (P4.9) and the kill-switch authority.
4. **Approval key material + rotation** (P6.4) and the principal registry's authority model (P4.3).
5. **MQTT in the first pilot** or defer to M4/M5 (P8.2).
6. **Retention windows** for the notification log and artifacts (P6.1, P8.3).

---

## 15. Cross-repo coupling notes

- **Tamoz** codes its worker against the **P1.4 conformance harness** and the **P1.1 frozen proto**;
  its snapshot-digest verification depends on **P0.1**; its outcome loop depends on **P6**. Keep the
  proto and the vectors file the single shared truth.
- **streams-simulator** is the natural **counterfactual/plant** for P7.2 and end-to-end tests; its
  `silent_no_effect`/`confirmed_no_effect` effector modes exercise P4.8 reconciliation and P5
  reconsideration. Before relying on it, verify its vendored `trace-record-v0.1` matches this repo's
  live ingress contract (a cheap check that closes a silent-drift risk).
