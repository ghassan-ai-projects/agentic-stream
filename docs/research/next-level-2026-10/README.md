# Taking Agentic Stream to the next level (2026-10-02)

Status: brainstorm and desk research. **Nothing here is authorized implementation.** Each
proposal that changes architecture, contracts, or a documented non-goal needs a design
change in [`docs/design/DECISIONS.md`](../../design/DECISIONS.md) first, as `AGENTS.md`
requires.

This package builds on, and does not repeat, the
[standalone product research (2026-09-24)](../standalone-product-2026/README.md). That
study asked *"is there a business?"* and answered *"run buyer discovery on real
traces"*. This one asks a different question: **what engineering, product, and ecosystem
moves would make the runtime clearly better and easier to adopt, so that discovery has
something stronger to show?**

## Read in this order

1. [01-STARTING-POINT.md](01-STARTING-POINT.md): what the project has proven, what the
   companion integration rounds found, and which findings are still open in the code.
2. [02-LANDSCAPE-2026.md](02-LANDSCAPE-2026.md): what changed in the world during 2026
   (Flink Agents, MCP Tasks, agent governance toolkits, the EU AI Act, deterministic
   simulation testing, the Unified Namespace) and what it means here.
3. [03-OPPORTUNITIES.md](03-OPPORTUNITIES.md): about 30 ideas across eight tracks, each
   with value, effort, invariant risk, and evidence.
4. [04-ROADMAP.md](04-ROADMAP.md): a sequenced plan in three horizons, with gates and
   kill criteria.
5. [05-SOURCES.md](05-SOURCES.md): external sources, with the date each was checked.
6. [06-OBSERVABILITY-INGRESS.md](06-OBSERVABILITY-INGRESS.md): follow-up on ingesting
   Prometheus, Kubernetes, Grafana, and logs through an external bridge, with proposed
   [ADR-015](../../design/DECISIONS.md#adr-015-proposed-external-ingress-bridges-over-an-acknowledged-socket).

## Executive summary

**Position.** In 2026 the market split. One camp puts *agents inside the stream*: Apache
Flink Agents reached 0.3.0 in June 2026, and Confluent's Streaming Agents are in open
preview. The other camp *governs agents' actions*: Microsoft's Agent Governance Toolkit
(April 2026) and the OWASP Agentic Top 10. Agentic Stream already sits in the gap between
them. It is the deterministic layer that decides **when** reasoning is warranted, **what
immutable evidence** it saw, and **whether** its proposal may still execute now that the
world has moved on. Nobody else ships that combination as one contract. The next level
is to make that layer **provable, authorable, connectable, and inspectable**, rather than
to grow it into a general agent platform.

**Direction (owner decision, 2026-10-02): Tamoz is always the brain.** Agentic Stream
owns time and authority, and Tamoz owns all judgment. No other agent framework, MCP
client, or direct-model executor is a production reasoner. The native deterministic
executor stays as a no-model baseline for tests and Class C comparisons. Proposals below
are aligned to this. Making it binding needs the proposed
[ADR-016](../../design/DECISIONS.md#adr-016-proposed-tamoz-is-the-sole-production-reasoner),
because ADR-003 currently says workers are Go-only and Tamoz's worker is Ruby.

**The eight bets, in priority order:**

| # | Bet | Why now | Size |
| --- | --- | --- | --- |
| 1 | **Close the known semantic gaps.** Add a correction-aware reducer (F-1), `on_time` completeness for late-settled windows (F-2), and virtual-time heartbeat timers (F-4). Then publish a measured single-node ceiling. | The integration rounds found these gaps and they are still open in the code. Every new domain hits F-1 again. | S–M |
| 2 | **A spec authoring toolchain:** `spec test` (golden Situation expectations against traces), `spec lint` (the known pitfalls), `explain`, and `spec diff` by counterfactual replay. | The cold-chain round needed **nine live spec corrections**. Authoring friction is the biggest adoption tax the lab has measured. | M |
| 3 | **An operator surface:** read-only query/explain API plus CLI for Situations, episodes, intents, approvals, quarantine redrive, and unknown-outcome reconciliation. | These capabilities exist only inside the runtime. Limitations call this out, and the previous study flags it as the main repo-to-product gap. | M |
| 4 | **SituationBench:** a public benchmark of late, duplicated, out-of-order, silent, adversarial, and corrected streams with sealed ground truth, built on the unified evaluation design. | No benchmark exists for agents acting on live streams. Owning the yardstick is cheap leverage, and it also implements `docs/eval/`. | M |
| 5 | **Deterministic simulation testing (DST):** one seed drives crashes, SQLite faults, worker loss, and effector timeouts in virtual time. | The runtime already has a virtual clock and deterministic replay. DST turns release blockers (crash, disk-full, recovery) into continuous evidence. | M–L |
| 6 | **Durable ingress acknowledgements plus an MQTT/Sparkplug B adapter.** Map birth/death certificates to source health. | UNS/Sparkplug is the dominant industrial integration pattern, and its lifecycle model matches the completeness semantics. The live socket has no per-event commit ack today. | M (needs a design change; MQTT is currently deferred) |
| 7 | **A decision-record evidence pack:** map `export-run` artifacts to EU AI Act Art. 12 logging, OWASP ASI risks, and CISA's AI-in-OT principles, then sign the artifacts. | Regulation and buyers now ask for this evidence. The runtime already produces most of it, so the remaining work is mapping, signing, and documentation. | S–M |
| 8 | **A larger operator vocabulary:** sequence/CEP ("A then B within T"), state duration, categorical latest, EWMA/percentile, and declared entity relations. | Round 4 (cascading neighbours) and Round 5 (string-valued modes) need constructs the three operator kinds cannot express. | M–L |

**What not to do next:** a web UI, a multi-agent mesh, distributed scale-out, a general
workflow engine, or putting LLMs or time-series foundation models in the hot path. Each
would dilute the one property competitors lack: deterministic, explainable authority over
agent actions on changing evidence. The design already defers all of these, and nothing
found in this research justifies reversing that.

## Method and limits

This package draws on three sources. The first is a read-only review of this repository
(code, docs, and `docs/eval/`). The second is a read-only review of the companion research
lab: integration rounds 1–5, the real-world sensor bench, the voice and coworker studies,
and the streams simulator design. The third is web research on primary or near-primary
sources, checked on 2026-10-02 (see [05-SOURCES.md](05-SOURCES.md)). Lab paths and names
are left out on purpose, following the repository's rule against personal paths and
private references.

No new benchmarks were run, no customers were interviewed, and no competitor was tested.
Where a claim comes from the lab, it is labelled **lab-reported**. Where it was
re-checked in this checkout, it is labelled **verified**.
