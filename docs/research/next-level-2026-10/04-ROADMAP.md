# 04 · Sequenced roadmap

This is a proposal, not a commitment. IDs refer to [03-OPPORTUNITIES.md](03-OPPORTUNITIES.md).
Items marked `decision` or `non-goal` need a DECISIONS entry before any code is written.

The sequencing rule is that **each horizon produces the evidence the next one needs.** No
connector before ingress acks. No new domain round before F-1 is fixed. No capability or
competitive claim before SituationBench and capacity numbers exist.

## Horizon 1: Credible core (about 4–6 weeks)

Goal: every claim on the status page is backed by a reproducible artifact, and an outsider
can author a working spec without a live debugging session.

| Order | Work | Exit evidence |
| --- | --- | --- |
| 1 | A3 virtual-time timer check, then A1 `prefer_corrected` reducer, then A2 completeness settling | The Round 5 cold-chain M6 trace flips its verdict to the ground-truth cumulative (~8.65h); every existing golden digest is unchanged |
| 2 | C2 `spec lint`, then C1 `spec test`, then C7 error messages | Each of the nine Round 5 spec corrections reproduces as a lint hit or a failing expectation |
| 3 | C3 `explain`, plus a read-only query API for Situations, episodes, intents, approvals, and outcomes | A reviewer reconstructs a full decision chain without SQL |
| 4 | A5 policy-mode enforcement and A7 status reconciliation | No schema-visible control lacks enforcement; `docs-check` catches CLI/doc drift |
| 5 | F1 DST harness (first cut: clock, ordering, worker crash, kill at commit) | A nightly seed sweep runs in CI; every failing seed reproduces locally |

**Gate to Horizon 2:** F-1, F-2, and F-4 closed; lint and test exist; DST is running.

## Horizon 2: Proof and openness (about 6–10 weeks)

Goal: publish numbers and contracts that others can reproduce and build on.

| Order | Work | Exit evidence |
| --- | --- | --- |
| 1 | A4 capacity ceiling and F4 soak | A published table showing *one node governs N entities at M events/s with p99 X*, the degradation behavior beyond it, and the soak report |
| 2 | H1 SituationBench v0 (5–8 scenarios, three baselines) on the unified `docs/eval` harness | The benchmark repo or directory, a verdict artifact per run, and Class D/Class C kept separate |
| 3 | F2 crash-point coverage, F3 backup/restore rehearsal, and E3 signed releases and run artifacts | Most `release_blockers` in release-status.json move to `implemented` with linked evidence |
| 4 | D1 durable ingress acks (design, then implementation) | A producer spool test shows zero loss and zero duplication across runtime kill/restart |
| 5 | E1 evidence pack and E2 OWASP ASI mapping, with the injection corpus as a standing test | Docs pages plus a test cell, worded as "supports", not "certifies" |
| 6 | ADR-016 decision, then D8 Situation Protocol v1 plus T1 cross-language conformance in CI, T5 Tamoz-down behaviour | The real Tamoz worker passes the kit in both repositories' CI; a Tamoz outage leaves deterministic processing healthy with no fabricated intents |

**Gate to Horizon 3:** the first tagged release with a compatibility policy, a published
capacity number, SituationBench v0, and the ingress ack contract.

## Horizon 3: Reach (about 1–2 quarters, sequenced by discovery results)

Choose the branch by what P0/P1 discovery from the
[previous study](../standalone-product-2026/FINDINGS.md#8-research-to-decision-experiment-program)
shows. Do not build both branches speculatively.

| If discovery favors… | Build | Then |
| --- | --- | --- |
| **Industrial / edge** | D2 MQTT/Sparkplug adapter (separate process on D1), B2 state-duration and categorical latest, B6 alarm-load accounting, D5 CMMS-draft effector, H2 physical demo | B4 entity relations for cascades (BESS), and F5 hot standby if a site requires it |
| **Software operations / SRE** | D5 webhook and incident-annotation effectors, B1 sequence operator, D6 OTel GenAI spans, D4 Flink/Kafka bridge | H4 comparison write-ups against Flink Agents and Temporal |
| **Tamoz-led operations (coworker / agent fleet)** | T2 evidence, approval relay, and outcome wiring; T3 learning contract; H5 agent-fleet dogfooding; C6 LLM-assisted authoring (offline) | G2 shadow spec canary and G3 offline tuning |
| **Either** | G1 outcome datasets and G4 cognition-economics report | B7 WASM operators only if two or more partners need domain math that the vocabulary cannot express |

## Kill and pivot criteria

- **Authoring tools do not reduce spec iteration.** If a new domain round with C1–C3
  still needs more than three live rewrites, the problem is the spec *language*, not the
  tooling. Open a design review of the SituationSpec model before adding operators.
- **SituationBench shows no gap.** If rules-only or LLM-per-event match Agentic Stream on
  Class D and Class C at similar cost, the temporal-authority thesis is weak for those
  scenarios. Publish the result honestly and narrow the claim.
- **The capacity ceiling is far below target workloads.** If one node cannot govern a
  realistic site (for example, under 500 entities at observed rates), profile and fix
  before any connector work. Do not jump to distribution.
- **The Tamoz seam keeps drifting.** If cross-repository conformance (T1) still breaks on
  more than one release in three after the kit lands, the seam is too wide. Shrink the
  protocol surface before adding features to either side.

## What this roadmap explicitly preserves

All ten invariants. The single-node SQLite posture. No broker, graph engine, or web UI in
core. Tamoz as the sole production reasoner over the gRPC worker protocol (pending ADR-016), with
no MCP or other-agent proposal surface. Domain data stays in JSON registries. Model access stays out of the
deterministic plane.
