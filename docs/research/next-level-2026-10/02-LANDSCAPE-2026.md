# 02 · The 2026 landscape and what it means here

This page covers only developments that change a decision for this project. The
[previous study](../standalone-product-2026/DEEP_DIVE.md) already covers the incumbent
SRE and industrial platforms in depth: PagerDuty, CloudWatch, Datadog, incident.io,
SiteWise, Azure IoT Operations, and Senseye. Source IDs (L1…) refer to
[05-SOURCES.md](05-SOURCES.md).

## 1. "Agents in the stream" became a product category

- **Apache Flink Agents** is a Flink sub-project for event-driven agents that run as
  operators inside a Flink job. It released 0.2.0 in February 2026 and 0.3.0 on
  2026-06-19. 0.3.0 added a YAML API with JSON Schema, Agent Skills, Mem0-based long-term
  memory, a *durable reconciler* for side effects during failure recovery, Fluss as an
  action-state store, and an EventLog view in the Flink web UI [L1, L2]. Alibaba Cloud,
  Ververica, Confluent, and LinkedIn back it [L3].
- **Confluent Streaming Agents** runs agents inside Confluent Cloud Flink pipelines, still
  in *open preview* [L4].

**Implication.** The primitive "call an LLM from a stream" is now commoditized and backed by
large vendors. Agentic Stream should **not** compete on throughput, connectors, or "agent as
operator". Its differentiators are things those projects document weakly or not at all:

| Concern | Agents-in-the-stream (as documented) | Agentic Stream |
| --- | --- | --- |
| When to reason | Every routed event can reach an agent operator | Deterministic cognitive scheduler with debounce, cooldown, coalescing, and budgets |
| What the model saw | Operator state at call time | An immutable, digested Situation version bound to the episode |
| Late or corrected evidence after a decision | Not a documented first-class concept | Correction → new version → reconsideration → pending-intent revalidation |
| Effect authority | Tools are called from inside the agent; a reconciler handles recovery | The model cannot call effectors; policy rechecks before dispatch; outbox and unknown-outcome states |
| Replay | Flink checkpoints and replay of its own state | Byte-level deterministic replay with effects disabled, plus shadow and counterfactual modes |

**Strategic consequence:** position Agentic Stream as **the supervisory and authority layer
downstream of any stream processor**, including Flink and Flink Agents. Flink's
change-detection output becomes Agentic Stream evidence. Do not position it as a rival
stream engine. See opportunity D4.

## 2. Agent protocols matured, and long-running work got a standard

- The **MCP 2026-07-28 specification** is the largest revision since launch. It adds a
  stateless core, an extensions framework, and **Tasks**, an official extension where
  `tools/call` returns a durable task handle that the client drives with `tasks/get`,
  `tasks/update`, and `tasks/cancel` [L5, L6].
- **Implication.** Tamoz is the sole reasoner, so MCP is **Tamoz's** integration surface
  for its own tools and skills. It is not a way for arbitrary agents to propose intents to
  Agentic Stream. Two narrow uses remain. First, a *read-only* MCP view of Situations and
  explanations for humans' assistants and operator tooling. Second, Tasks' lifecycle
  (pending, running, cancelled, completed) is a useful reference when hardening the
  Tamoz-facing intent and approval contract. The streams-simulator design already reached
  the right rule: use MCP for control, never for event transport, because MCP push is
  best-effort.

## 3. Runtime governance became a named category

- **Microsoft Agent Governance Toolkit** (MIT, 2026-04-02) intercepts tool calls and
  messages and evaluates deterministic allow/deny policy (YAML, OPA/Rego, Cedar). It
  claims coverage of all 10 OWASP Agentic risks [L7].
- The **OWASP Top 10 for Agentic Applications 2026** (ASI01–ASI10, published Dec 2025)
  covers goal hijacking, tool misuse, rogue agents, cascading failures, and more [L8].
- **Implication.** The industry now accepts "deterministic policy outside the model" as
  table stakes. AGT governs *calls*. It does not govern *time*: whether the evidence
  behind a call is still current, complete, and uncorrected. That temporal revalidation
  is Agentic Stream's distinctive governance claim, and it should be stated in OWASP ASI
  terms (opportunity E2). AGT-style policy engines can also be *complementary*: they
  guard the reasoner's own tools inside an episode, while Agentic Stream guards effects.

## 4. Regulation made decision records a requirement

- **EU AI Act Art. 12** requires high-risk systems to *automatically* record events over
  their lifetime, covering risk situations, post-market monitoring, and deployer
  operational monitoring. High-risk obligations apply from **2026-08-02**. A
  Digital-Omnibus delay to December 2027 had passed Parliament but was not yet adopted by
  Council at the time of the cited source [L9]. Re-check before relying on either date.
- **CISA, ASD's ACSC, and partners** published *Principles for the Secure Integration of
  AI in OT*: understand AI, assess its use in OT, establish governance, and embed safety
  and security with operator oversight and incident-response integration [L10]. NIST SP
  800-82 Rev. 3 remains the OT baseline, and a Rev. 4 draft appeared in September 2026
  (see the previous study).
- **Implication.** The runtime already produces Art. 12-shaped logs: immutable Situation
  versions, episode bindings, decisions, policy verdicts, and outcomes. Mapping, signing,
  and documenting them (opportunity E1) is low effort and high credibility, especially
  for OT buyers. This is not a compliance certification, and the docs must say so.

## 5. Observability standards for agents are still moving

- The OpenTelemetry GenAI semantic conventions (model, agent, tool, and MCP spans) moved to
  a dedicated repository in June 2026. They are all still at **Development** stability,
  with an opt-in switch for version transitions [L11].
- **Implication.** Emit `gen_ai.*` attributes on episode and tool spans behind the opt-in
  flag, and keep the runtime's own low-cardinality metrics as the contract. This is
  cheap interoperability with Datadog, Grafana, and other backends. Do not depend on
  attribute names yet.

## 6. Deterministic simulation testing went mainstream

- DST was pioneered at FoundationDB and is used by TigerBeetle's VOPR (about 1000× wall
  speed, fuzzing seeds on 1024 cores) and Antithesis. In 2026 most serious database
  companies use or evaluate it. A Go `detsim` package (August 2026) offers a virtual-time
  scheduler with network and storage fault injection [L12, L13].
- **Implication.** Agentic Stream is unusually well suited to DST because time is already
  virtualized and state changes are serial per partition. A seeded fault simulator would
  turn three release blockers into continuously generated evidence: crash/unclean
  shutdown, disk-full, and recovery and owner takeover (opportunity F1). It is also a
  credible public differentiator: *"every release survives N million simulated crashes"*.

## 7. Industrial integration converged on the Unified Namespace

- **UNS** uses an MQTT broker hub with an ISA-95 topic hierarchy. **Sparkplug B** (Eclipse)
  adds a standard topic namespace, typed Protobuf payloads, and **birth/death
  certificates** for device lifecycle state [L14, L15].
- **Implication.** Sparkplug's birth/death/rebirth model maps directly onto Agentic
  Stream's source-health, completeness, and gap semantics. A device death certificate is
  an explicit *absence* signal, which most consumers ignore. For the industrial lane,
  this is the single adapter that would reach the most installed base. MQTT ingress is
  currently *deferred by design*, so reopening it needs a recorded design decision (see
  [04](04-ROADMAP.md)).
- **ISA-18.2 / EEMUA 191** define an *alarm flood* as ≥10 alarms per 10 minutes per
  operator, with a steady-state target of about 1 per 10 minutes [L16]. This gives the
  cognitive scheduler a recognized external yardstick: measure operator-facing
  Situations and intents per operator-hour against ISA-18.2 targets (opportunity B6).

## 8. Time-series foundation models are useful, but not in the hot path

- Zero-shot TSFMs (Chronos-2, TimesFM 2.5) show competitive anomaly-detection F1 in
  domain studies, for example drilling anomalies at ICML 2026 [L17].
- **Implication.** These models are non-deterministic across hardware and versions, so
  they cannot run inside the deterministic plane. The right shape is a **sidecar that
  emits scores as ordinary evidence events**, carrying model and version identity and
  quality flags. Replay then consumes the recorded scores (opportunity B5). This keeps
  invariants 1 and 4 intact and lets users bring their own detector, including SiteWise
  or Senseye outputs.

## 9. Durable execution got very well funded

- Temporal raised a $300M Series D in February 2026 at a $5B valuation and is positioning
  durable execution as agent infrastructure [L18].
- **Implication.** "Durable agent workflows" is a crowded and well-capitalized position.
  Agentic Stream should integrate with it, for example by letting an effector start a
  Temporal workflow with the intent's idempotency key, and should not compete with it.

## Net reading

The world converged on two halves: agents *in* streams, and policy *around* agent calls.
The half that remains unowned is **temporal authority**: knowing that the evidence behind
a decision is still true, complete, and uncorrected at the moment of effect, and being
able to prove it later. That is Agentic Stream's thesis. The work now is to make it easy to
adopt next to those ecosystems.
