# 03 · Opportunity catalog

This is a brainstorm across eight tracks. Every item carries:

- **Value**: what it unlocks, scored H/M/L.
- **Effort**: S = days, M = 1–3 weeks, L = more than a month, all for one experienced
  engineer with agents.
- **Design**: whether it fits the documented design (`fits`), needs a DECISIONS entry
  (`decision`), or reverses a documented non-goal (`non-goal`).
- **Invariant risk**: which of the ten invariants it could weaken if done carelessly, and
  the guardrail that prevents that.

Ideas already recommended by the [previous study](../standalone-product-2026/FINDINGS.md),
such as buyer discovery and paid pilots, are not repeated here. They remain the commercial
track that this engineering work feeds.

---

## Track A: Close the proof

| ID | Idea | Value | Effort | Design |
| --- | --- | --- | --- | --- |
| A1 | **A `prefer_corrected` reducer strategy (F-1).** The winning value is chosen by completeness and correction lineage (corrected > on_time > provisional, then emission watermark), not by observation event time. This is additive: `latest_event_time` keeps its semantics. | H | S–M | decision (spec enum) |
| A2 | **Window completeness settling (F-2).** `early_and_close` windows emit `on_time` once the watermark passes window end plus allowed lateness, so gated intents stop binding provisional versions by accident. | H | S–M | decision |
| A3 | **Virtual-time timers in live mode (F-4).** Confirm or fix whether heartbeat timers advance on virtual time. Add a replay test that an absence detected live produces the same Situation history on replay. | H (invariant 4) | S | fits |
| A4 | **Publish the single-node ceiling.** Run the scaffolded Round 4 plan: a k8s or host-health throughput pre-test, then BESS 256 → 512 → 1024 racks. Record events/s, entities, p99 transition latency, scheduler queue depth, SQLite WAL size, and the point where heartbeat windows go stale. Publish it as a capacity table. | H | M | fits |
| A5 | **Finish policy-mode enforcement.** Make `deny` and `simulate` real with dedicated tests, or remove them from the schema until they are. No control should be visible in the schema without being enforced. | M | S–M | fits |
| A6 | **Close the release blockers** in [release-status.json](../../../documentation/governance/release-status.json): backup/restore rehearsal, disk-full, unclean shutdown, soak, compatibility policy, and signed artifacts. F1 and F3 below generate most of this evidence. | H | M–L | fits |
| A7 | **Reconcile status drift.** Bring status, limitations, and release-status up to date with live UDS ingress and the effect profiles. Add a `docs-check` rule that fails when a `cmd/` subcommand is missing from the CLI reference (`export-run` and `verify-run` were). | M | S | fits |

**Invariant risk.** A1 and A2 change Situation content for existing specs only if those
specs opt in. Golden replay digests must stay unchanged for every checked-in spec. Treat
that as the acceptance test.

## Track B: Expressiveness (what a SituationSpec can say)

Each new operator is pure, deterministic, and replayable, with no I/O, no wall time, and
no maps iterated without ordering.

| ID | Idea | Value | Effort | Design |
| --- | --- | --- | --- | --- |
| B1 | **Sequence/CEP operator:** "A then B (not C) within T, per entity", with event-time semantics and explicit late-arrival handling. This covers door-open-then-temp-rise, deploy-then-error-spike, and failed-auth-then-success-from-new-host. | H | M–L | decision |
| B2 | **State-duration operator:** "value in state S for ≥ T", for example mode = `cooling` for over 20 minutes. Use it with a **categorical latest** aggregate (F-3) for strings and enums. | H | M | decision |
| B3 | **More numeric aggregates:** EWMA, percentile (deterministic sketch with fixed parameters), count-distinct (exact up to a bound), and rate of change. | M | S–M | decision |
| B4 | **Declared entity relations:** a static, versioned topology in the spec, such as `rack-007 neighbours [rack-006, rack-008]` or `clarifier-2 downstream_of aeration-1 lag 2h`. Operators and triggers can then reference related entities' published Situation versions *by version*, never by live state. This is the minimum needed for BESS cascade and wastewater multi-hop causality without a graph engine. | H | L | decision (needs a careful partitioning design; cross-partition reads must stay snapshot-based) |
| B5 | **External detector evidence:** a documented pattern and schema for sidecar scorers (TSFMs such as Chronos or TimesFM, SiteWise or Senseye anomaly outputs, Flink jobs). They emit `*.anomaly_score.observed` events with model, version, and quality fields. Replay uses the recorded scores, so no model runs inside the engine. | M–H | S (pattern) / M (example sidecar) | fits |
| B6 | **Alarm-load accounting:** first-class metrics for operator-facing notifications and intents per operator per 10 minutes, with flood detection by ISA-18.2 thresholds. The scheduler can then expose "this spec would have flooded the operator N times last week". | M | S–M | fits |
| B7 | **WASM user-defined operators** (wazero, pure Go, zero CGO). These are deterministic by construction: no host imports for time, randomness, or I/O, a fuel limit, and the module digest included in the spec digest. They are the escape hatch for domain math without forking the engine. | M | L | decision (new dependency, needs justification) |

**Guardrail.** Do B1–B3 before B4 and B7. Each lab round should name the construct it
lacked before a new operator is added. Do not add operators speculatively.

## Track C: Authoring experience (the biggest measured adoption tax)

| ID | Idea | Value | Effort | Design |
| --- | --- | --- | --- | --- |
| C1 | **`agentic-stream spec test`:** a spec ships with `*.expect.yaml` files that assert Situation lifecycle points, feature values, admitted episodes, and intents at given event times for a given trace. The runner uses deterministic replay. Think of it as table-driven tests for situations. | **H** | M | fits |
| C2 | **`spec lint`:** static checks for known foot-guns, each with a fix hint. Examples: debounce longer than the heartbeat interval; `maxOutOfOrderness` too wide for an `on_time` gate; a trigger that reads a feature with no reducer (silent default); `materialDelta` that flaps on provisional/on_time; wall-clock assumptions; and R2+ intents bound to `any` completeness. Every lint comes from a real lab finding. | **H** | S–M | fits |
| C3 | **`explain` timeline:** given a Situation ID, print the causal chain of evidence, feature changes, versions, admission decisions (including *why not admitted*), episode, decision, policy verdict, and outcome. This uses existing durable records and is read-only. | H | M | fits |
| C4 | **`spec diff --trace`:** run two spec versions over the same trace by counterfactual replay, then report changed versions, admissions, intents, and estimated model calls. This is change-impact analysis before deploying a new threshold. | H | M | fits (replay already supports counterfactual) |
| C5 | **`spec scaffold --from-trace`:** infer event types, entities, value ranges, cadence, and gaps from a sample trace, then emit a commented starter spec plus an expectations stub. | M | M | fits |
| C6 | **Offline LLM-assisted authoring:** a documented workflow, outside the runtime, where a coding agent drafts or edits a spec, and the compiler, C2 lint, C1 tests, and C4 diff act as the oracle. The model never enters the runtime. A Claude Code or Codex skill file in the repo could teach it. | M–H | S | fits |
| C7 | **Better compiler errors:** JSON Schema paths translated into spec-language messages with "did you mean" suggestions, plus a published editor schema (`$schema` URL) for YAML LSP completion. | M | S | fits |

**Why first.** Round 5 needed nine live spec rewrites. Each one would have been a lint or a
failing `spec test` caught in seconds instead of found during a live run.

## Track D: Connectivity and ecosystem

| ID | Idea | Value | Effort | Design |
| --- | --- | --- | --- | --- |
| D1 | **Durable ingress acknowledgement contract:** per-event (or per-batch) commit acks with dedup identity on the live socket, plus an admission-query endpoint. Producers delete from their spool only after an ack. This is a prerequisite for every real adapter. | **H** | M | decision (contract change) |
| D2 | **MQTT + Sparkplug B ingress adapter.** Map NBIRTH/DBIRTH/NDEATH/DDEATH to source-health and gap records, metric aliases to the event-schema registry (data, not Go literals), and topic hierarchy to entity identity. The adapter can be a separate process speaking D1, which keeps the core broker-free. | H (industrial) | M | **non-goal today:** MQTT is deferred. A separate adapter process feeding D1 respects "no broker in core". |
| D3 | **~~MCP server (read and propose)~~, revised: read-only MCP view.** Revised because Tamoz is the sole reasoner: no `propose_intent` for arbitrary agents. Keep only read-only Situation and explanation resources for operator assistants. Original text:  Situations, versions, and explanations exposed as MCP resources; one tool, `propose_intent`, returns an **MCP Task** that mirrors the intent lifecycle (pending approval → dispatched → reconciled/unknown/expired). Any MCP agent can then be a reasoner without a gRPC worker and without effector credentials. | **H** | M | decision (new public surface; proposals are untrusted input, so policy applies exactly as for worker intents) |
| D4 | **"Downstream of Flink/Kafka" reference integration:** a documented pattern and example where a Flink (or Flink Agents) job emits normalized change events to Agentic Stream through a small Kafka→D1 bridge. This positions the project as complementary to the stream ecosystem. | M | M | fits if the bridge stays outside core |
| D5 | **Reference effectors with real reconciliation:** (a) a generic webhook with an idempotency-key header and a status-query callback; (b) a ticket/CMMS *draft* creator; (c) an incident annotation (PagerDuty or incident.io event API). Each needs documented unknown-outcome reconciliation. These are the "reversible, non-safety effect" that P4 in the previous study needs. | H | M per effector | fits |
| D6 | **OTel GenAI spans:** `gen_ai.*` attributes on episode, model, and evidence-tool spans behind `OTEL_SEMCONV_STABILITY_OPT_IN`, with a trace link from source event to Decision to Outcome. Round 2 already tested traceparent propagation. | M | S | fits |
| D7 | **CloudEvents mapping** for ingress and notifications (attribute mapping only; the envelope stays canonical). | L–M | S | fits |
| D8 | **Publish the Situation seam as a versioned spec:** "Situation Protocol v1". It would cover the envelope, snapshot digest, Decision and Intent, the approval relay, and outcome feedback, plus a **conformance kit** (vectors and a test runner) that other reasoners such as Tamoz or a LangGraph agent can pass. This turns a private integration into a pinned contract. **With Tamoz as the sole brain, the kit's job is to keep Agentic Stream and Tamoz from drifting.** Run it in both repositories' CI against the real Ruby worker, not only Go fakes. | **H** | M | decision (see ADR-016) |

## Track E: Trust, governance, and compliance

| ID | Idea | Value | Effort | Design |
| --- | --- | --- | --- | --- |
| E1 | **Decision-record evidence pack:** map `export-run` artifacts to EU AI Act Art. 12(2) categories, ISO/IEC 42001 operational records, and CISA AI-in-OT principles. Add retention guidance. Explicitly *not* a certification. | H | S–M | fits |
| E2 | **OWASP ASI threat mapping:** a table from each ASI01–ASI10 risk to the invariant, mechanism, and test that addresses it, or to "out of scope / deployment responsibility". Pair it with the auth-EDR injection corpus from Round 5 as a standing test cell. | H | S–M | fits |
| E3 | **Signed artifacts:** sign release binaries (Sigstore/cosign, SLSA provenance, SBOM, which are already release blockers) and **sign run artifacts** (`export-run` manifest digest signed by the runtime key), so a reviewer can verify a decision record offline with `verify-run`. | M–H | S–M | fits |
| E4 | **Approvals as signed assertions:** standardize the ed25519 approval assertion proven in Round 2 as the approval contract. It should cover the approver identity, the intent digest, the Situation version, and expiry. An approval delivered on any channel (CLI, Tamoz, Slack, phone) is valid only if the signature verifies against a configured approver key. | H | M | decision |
| E5 | **Policy-as-data review tooling:** `policy explain <intent>` shows which rule allowed or denied it, against which Situation version, under which approval. Optionally, offer Cedar or OPA *export* for organizations that standardize on those, while CEL remains the evaluated source. | M | M | fits |

## Track F: Reliability engineering

| ID | Idea | Value | Effort | Design |
| --- | --- | --- | --- | --- |
| F1 | **Deterministic simulation testing harness:** one seed drives the virtual clock, input ordering and delays, worker crashes and timeouts, effector accept-then-lose-response, SQLite I/O errors (via a VFS shim or fault-injecting `storage` wrapper), and process kill at any commit boundary. Assertions are the ten invariants plus "replay of the surviving log equals live history". Run it continuously in CI with a nightly seed sweep. | **H** | M–L | fits (test-only) |
| F2 | **Crash-point coverage:** enumerate every durable commit boundary (outbox, lease, fence, version publish) and prove recovery from a kill at each one. F1 can sample these. | H | M | fits |
| F3 | **Backup, restore, and DR:** document and rehearse SQLite online backup, and optionally Litestream continuous WAL shipping to object storage, with point-in-time restore. Prove that a restored runtime resumes with no duplicate effects, because the outbox is idempotent and unknown outcomes block. | H | M | fits (Litestream is an external sidecar, not a dependency) |
| F4 | **Soak with fault schedule:** 24–72 hours driven by streams-simulator with a declared fault schedule. Watch WAL growth, memory, scheduler fairness, and notification lag. Publish the report next to A4. | H | M | fits |
| F5 | **Hot standby (later):** a warm replica fed by WAL shipping, with fenced owner takeover. Ownership and fencing already exist. This is the step before any multi-node discussion. | M | L | decision |

## Track G: Learning loop (without breaking determinism)

| ID | Idea | Value | Effort | Design |
| --- | --- | --- | --- | --- |
| G1 | **Outcome-labelled datasets:** export reconciled outcomes joined to their Situation versions and decisions as an evaluation dataset. These are the ground truth that discovery (P1/P2) and Class C evaluation need. | H | S–M | fits |
| G2 | **Shadow spec canary:** run candidate spec version N+1 in shadow next to live version N on the same input. Diff Situations and intents continuously, then promote with evidence. This is C4 made continuous. | H | M | fits (shadow mode exists) |
| G3 | **Offline threshold and admission tuning:** use G1 data and counterfactual replay to search debounce, cooldown, and threshold values that minimize admissions at fixed recall. The output is a *proposed spec change* that goes through C1–C4. Learning stays offline; deployment stays deterministic config. | M–H | M | fits |
| G4 | **Cognition economics report:** per spec, report model calls per entity-day, cost per approved outcome, suppressed admissions, and the deterministic-only baseline. This makes the selective-cognition claim measurable, as `docs/eval` Class C requires. | H | S–M | fits |

## Track T: Tamoz as the brain (added 2026-10-02)

These follow from the owner decision that Tamoz is always the reasoner. Status of Tamoz
internals is lab-reported, so re-verify against the Tamoz repository before planning.

| ID | Idea | Value | Effort | Design |
| --- | --- | --- | --- | --- |
| T1 | **Cross-language conformance in CI.** Agentic Stream's worker conformance suite runs against the real Tamoz Ruby worker (pinned version) on every change to either repository: handshake, features, fencing, budgets, cancellation, digests, and terminal validation. Today the suites use Go implementations only. | **H** | M | decision (ADR-016) |
| T2 | **Close the seam features Tamoz does not use yet.** Lab reports flagged an unwired evidence-tool socket in Tamoz (`SUPPORTED_FEATURES=[]` at Round 3), and a tested but unwired `ApprovalRelay` and `OutcomeSubscriber` path in the voice study. Wire all three so Tamoz can pull recorded evidence, relay approvals, and learn from reconciled outcomes. | **H** | M (mostly Tamoz-side) | fits |
| T3 | **Outcome → Experience as the learning contract.** Agentic Stream's reconciled, independently observed outcome is the *only* source that may create an "observed" Experience in Tamoz (proven in Round 1). Make `reconciliation_version` corrections update rather than duplicate Experiences, and test it across repositories. | H | S–M | fits |
| T4 | **One episode contract for all lanes.** Fast and deep lanes and `reconsider` episodes all go to Tamoz, and lane selects Tamoz's reasoning depth and budget. A small, cheap model is Tamoz's choice for the fast lane, not a second brain. | M–H | S | fits |
| T5 | **Tamoz-down behaviour.** When Tamoz is unavailable, episodes fail closed: deterministic rules and watch conditions keep running, no intent is fabricated, and a source-health-style "reasoner unavailable" record is published. This makes "the brain is always Tamoz" safe under its outages. | **H** | S–M | fits |
| T6 | **Local-model Tamoz for air-gapped OT.** Run Tamoz with a local OpenAI-compatible model (Ollama, vLLM) for sites that cannot send plant data to a cloud model. Agentic Stream does not change. | M | S (Tamoz config + test) | fits |

## Track H: Proof, community, and positioning

| ID | Idea | Value | Effort | Design |
| --- | --- | --- | --- | --- |
| H1 | **SituationBench:** an open benchmark built from streams-simulator domains with sealed generative ground truth. It covers Q1–Q8 stressors: gross lateness, duplicates, out-of-order data, silence, churn, an adversarial source, an injection payload, and a correction after a decision. Scores follow the `docs/eval` philosophy: Class D gates, then Class C with `k ≥ 3`, a deterministic baseline, cost, and claim tier. Publish baselines for rules-only, LLM-per-event, and Agentic Stream. | **H** | M | fits (implements `docs/eval/`) |
| H2 | **The physical demo:** a 3-minute recorded run on the Arduino bench. A sensor drifts, a late correction arrives, a pending intent is revalidated and rejected, the LED and fan effect is verified by independent observation (G4a), and replay shows the identical decision record. It needs independent-instrument evidence to claim the `physical` tier. | H | S–M once G4 gates close | fits |
| H3 | **Public docs site and "first Situation in 10 minutes":** `go install` or a release binary, then a bundled demo trace, then `explain`, all without a model. A deterministic-only mode is a feature, not a fallback. | M–H | S–M | fits |
| H4 | **Comparison write-ups with code:** the same scenario built with Flink Agents, with Temporal plus an agent, and with Agentic Stream, scored on SituationBench, with LOC, failure-mode behavior, and replayability. This is honest and reproducible, and it tests the project's own thesis. | H | M–L | fits |
| H5 | **Agent-fleet dogfooding:** use Agentic Stream to supervise coding-agent fleets. Agent run events become Situations such as "stuck", "looping", "budget burn", or "flaky gate", with governed intents like pause, escalate, or open an issue. The Round 5 agent-fleet pick and the lab's loop-mechanics catalog already describe this domain. It is a software-only, high-volume, real-data domain the team lives in every day. | M–H | M | fits |

---

## Ideas considered and rejected (for now)

| Idea | Why not now |
| --- | --- |
| Web UI / console | A documented non-goal. C3, D3, and an SSE feed give operators and existing tools what they need first. Revisit only when a design partner's operators cannot work from the CLI, MCP, or their incident tool. |
| Multi-agent orchestration inside episodes | It belongs to Tamoz. The lab's own topology runs solved fewer tasks with delegation enabled. |
| LLM or TSFM inside operators | Breaks invariant 4. Use B5 sidecars instead. |
| Distributed partitions / Kafka-native engine | No capacity evidence yet that a single node is insufficient. Do A4 first. |
| Python workers, other agent frameworks, or MCP clients as reasoners | Tamoz is the sole production brain (owner decision, ADR-016 proposed). Other frameworks enter only *inside* Tamoz if Tamoz chooses them. |
| OpenAI-compatible native executor as a production brain | It is demoted to a test and baseline path. The deterministic provider remains the no-model baseline. |
| General ExecutionRail / robot control | The previous study rejected it for v1. Keep executors external and consume receipts as evidence. |
| Vector memory / RAG in runtime | That is Tamoz's memory (Experience, Knowledge, Wisdom), per the Situation-seam ownership table. |
