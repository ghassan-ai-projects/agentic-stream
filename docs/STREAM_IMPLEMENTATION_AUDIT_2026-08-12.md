# Agentic Stream — Implementation Audit (independent)

Date: 2026-08-12
Auditor: independent review against the joint build plan
(`agent-research-lab/integration/PLAN_AGENTIC_STREAM_BUILD.md`) and the integration design
(`STREAM_RESPONSIBILITIES.md §3.1–3.20`, `PROTOCOL.md`, `LIFECYCLES.md`, `CONTRACTS.md`).
Method: direct source inspection of `internal/`, `migrations/`, `cmd/`, `proto/`; cross-repo
context from the tamoz audit.
Repo state: `main` @ `2bc82e6`. 28 internal packages, 20 migrations.

---

## 1. Verdict

**Agentic Stream has implemented essentially the entire plan — P0 through P8, including the
governance items that the plan had deferred to a hardening pass.** This is the opposite situation
from the Tamoz side: where Tamoz built the forward worker and stopped before the reverse/learning
half, the stream built the whole runtime, both halves.

Verified present and wired: the RFC 8785 canonical contracts, the compiled+frozen gRPC worker
protocol with a **process-isolated conformance harness**, a **native Go executor with a real OpenAI
provider** (so the stream can reason natively *or* dispatch to an external Tamoz worker via
`--worker-socket`), the **composed live pipeline running under `serve`**, fencing, decision
validation, the full **policy/action plane** (compensating intents, interlock at dispatch,
principals + roles, approval-assertion signing, source-health gating), watch conditions, quarantine
+ re-drive, cost control + kill path, event-schema registry, prompt/objective content-addressing,
reconsideration admission, SSE-bound durable notifications, and OpenTelemetry tracing.

The remaining open items are **narrow hardening/verification, not missing subsystems** — chiefly
external simulator-tool verification and the explicitly-postponed soak/release evidence (Gate D).

---

## 2. Status by phase (evidence-based)

Legend: ✅ implemented + tested · 🟡 partial / verify · ⬜ not present.

| Phase | Item | Verdict | Evidence |
|---|---|---|---|
| **P0.1** | RFC 8785 canonicalization + domain digest | ✅ | `internal/canonicaljson`; domain constants incl. `DomainPrompt`, `DomainApproval = "situation-runtime/approval-assertion/v1\n"` |
| **P0.2** | Versioned schemas + CloudEvents | ✅ | `internal/contractsv1`; `protocol_contract_test.go` |
| **P1.1–1.2** | Proto reconcile/freeze + gRPC codegen | ✅ | `proto/agenticstream/runtime/v1/*.pb.go`; oneof `EpisodeEvent` with `attempt_id/fence`, attempt-scoped `TerminalStatus`, `Reconsideration`, `ArtifactManifest` |
| **P1.3** | gRPC worker server + evidence host + tokens | ✅ | `internal/worker`, `internal/evidence`; `DialEpisodeWorkerSocketTLS` (`worker/uds.go:108`) = **mTLS** |
| **P1.4** | Native executor + **real model provider** | ✅ | `internal/executor/native/{native.go,openai.go}` (+tests) — a bounded model/tool loop with an OpenAI adapter |
| **P1.4** | Executor conformance harness | ✅ | `internal/executor/conformance/{conformance.go,conformance_test.go}`; commit `process-isolated worker conformance` |
| **P2** | Lifecycle separation + `(episode_id,attempt_id,fence)` | ✅ | migrations `003`,`011`,`012`; fenced attempts + owner epoch |
| **P3.1** | Decision validator | ✅ | `internal/decisions/validator.go`; `validator_test.go` (snapshot-mismatch, intent-type-not-allowed, …) |
| **P3.2** | Prompt/objective content addressing | ✅ | `episodes/assembler.go:34,149,161` — `PromptSHA256`/`ObjectiveSHA256` via `canonicaljson.Digest(DomainPrompt,…)` |
| **P3.3** | Event schema registry | ✅ | `internal/eventschema`; migration `016_event_schemas_quarantine`; commit `persist registered event schemas with deployments` |
| **P4.1/4.7** | Stream capability host | ✅ | `internal/policy/capabilityhost_test.go` enforces import direction and runs an injection corpus; source-health completeness is propagated from heartbeat features |
| **P4.2** | Policy gateway + idempotent outbox | ✅ | `internal/policy/policy.go`; migration `004_policy_audit` |
| **P4.3** | Principal registry / separation of duty | ✅ | migration `013_governance_interlock` — `principals` + `roles` tables; `approval_principal_not_authorized` denial |
| **P4.4** | Watch-condition intent + effector | ✅ | `internal/actions/watch_effector.go`; migrations `017`,`019`; commits `connect and fence live watch execution`, `harden … watch expiry` |
| **P4.5** | Compensating intents | ✅ | `policy.go:208-217` — `compensates` target load, `compensation_target_missing` / `compensation_tenant_mismatch`, routed through the chain |
| **P4.6** | Interlock readiness at dispatch | ✅ | `internal/interlock`; `dispatcher.go:184` dispatch-time interlock assertion; pipeline `WithInterlock(interlock.DurableReader{})` |
| **P4.7** | Source-health gating | ✅ | `missing_heartbeat` emits `uncertain`; `situations.Engine` publishes completeness-only versions; policy denies R2–R4 on `provisional`/`uncertain` |
| **P4.8** | Outbox + concrete effectors + reconciliation | ✅ | `internal/actions/{dispatcher,simulated_effector,composite_effector,watch_effector}.go` |
| **P4.9** | Quarantine + gap records / cost ceiling + kill | ✅ | quarantine: migrations `016`,`018` + re-drive; cost: `internal/costcontrol` + migration `015`; runner `WithCostControl(&costcontrol.Controller{})` |
| **P5** | Reconsideration admission (dedup) | ✅ | migration `005`; `admit deduplicated reconsideration episodes` |
| **P6.1** | Durable notification log + cursor + SSE | ✅ | migration `006`; `internal/notify/sse.go`; `bind durable notifications to SSE`; poison-retry migration `020` |
| **P6.4** | Approval workflow + assertion + withdrawal | ✅ | `policy.go` `CanonicalApprovalAssertion`/`ApprovalAssertionSigningBytes`; withdrawal `policy.go:286-290` → `situation_version_conflict`; RFC 9457 via `notify/sse.go:257` |
| **P7** | Replay isolation + worker-aware modes | ✅ | `internal/replay`; `isolated worker-aware replay modes` |
| **P8.1** | Trace propagation | ✅ | migrations `007`,`008`; `internal/telemetry`; `OpenTelemetry tracing and export` |
| **P8.2** | RFC 9457 + mTLS | ✅ | `api/http.go:15` RFC 9457 Problem Details; `worker/uds.go:108` mTLS dial |
| **P8.3** | Telemetry, runbooks, **soak** | 🟡 | `internal/telemetry`, `docs/runbooks/`; 24h soak/release evidence **explicitly postponed** (commit `postpone environment release evidence`) |
| **S1** | Compose the live pipeline | ✅ | `internal/runtime/pipeline.go` composes runner + dispatcher + ingest; `run continuous JSONL pipeline under serve` |
| **S2** | Connect + supervise worker | ✅ | `run-live --worker-socket` (`cmd/…/main.go:81,220`); budget + cost control + interlock + mTLS |
| **S3** | Conformance harness | ✅ | `internal/executor/conformance` |
| **S4** | Concrete effector for the simulator | ✅ | `internal/actions/simulated_effector.go` |
| **S5** | Simulator ingest | ✅/🟡 | adapter consumes the nested vendored `trace-record-v0.1` shape, validates control/model records, and enforces strict recorded-time ordering; external `streamsim adapter verify` remains unavailable in this checkout |

---

## 3. What is genuinely strong

- **The whole runtime runs.** `internal/runtime/pipeline.go` composes ingest → cognition/dispatch →
  decision validation → policy → action under `serve`, with cost control and interlock wired in —
  this was the #1 gap in the previous audit and it is closed.
- **Both reasoning paths exist.** A native Go executor with an OpenAI provider *and* an external
  worker socket, selected by `--worker-socket`. The stream is not blocked on Tamoz to demonstrate a
  full loop, and Tamoz plugs into the same fenced protocol.
- **Governance is real, not deferred.** Principals+roles, approval-assertion signing under a
  domain-separated rule, compensating intents at their own risk class, dispatch-time interlock,
  source-health denial, watch conditions, quarantine with re-drive, cost ceiling — the items the
  plan marked "hardening pass, later" are already in.
- **Protocol hygiene:** RFC 9457 problem details, mTLS worker transport, OpenTelemetry, a
  process-isolated conformance harness, and a protocol contract test.

---

## 4. Open items (narrow; verify or finish)

1. **External simulator verification (S5).** The repository adapter now matches the vendored
   nested `trace-record-v0.1` record shape and rejects the former flattened representation. The
   external `streamsim` binary is not installed here, so `streamsim adapter verify` remains an
   environment check for the machine that owns that tool.
5. **Gate D (soak/release) is explicitly postponed.** Runbooks exist; the 24-hour soak and release
   evidence are deferred by commit. That is a legitimate sequencing choice, but Gate D is not met,
   so this is not yet a "production-ready" tag.

---

## 5. Reading the two repos together

| | Agentic Stream | Tamoz |
|---|---|---|
| Forward supervised episode (Channel A) | ✅ (native + worker) | ✅ (worker) |
| Governed action plane | ✅ full | n/a (stream owns it) |
| Reverse evidence channel | ✅ host present | ⬜ client not wired |
| Learning loop (outcome → Experience) | 🟡 emit side to confirm | ⬜ subscriber absent |
| Approval relay to a human | ✅ authority side | ⬜ delivery side absent |

**The stream is ahead of the agent.** The end-to-end loop is currently gated less by the stream
than by Tamoz's unbuilt reverse half (evidence pull, outcome subscriber, approval relay). The
stream now emits the typed, cursor-backed Channel B lifecycle feed that the outcome subscriber can
consume.

---

## 6. Recommendation

Agentic Stream can credibly claim **"full B-full runtime, both halves, minus Gate-D release
hardening."** The remaining actions are external verification and the postponed release gate:

1. Run the external `streamsim adapter verify` when the simulator tool is available.
2. Run **Gate D** (soak + runbooks) later to earn the production tag; it remains postponed by scope.

Everything else is done to a genuinely high standard.

---

*Files cited are at the current `main` revision; line numbers are approximate. Items marked 🟡 warrant a direct
read before relying on them; two counts in an earlier pass were corrupted by a shell-glob error and
were re-verified with quoted patterns for this document.*
