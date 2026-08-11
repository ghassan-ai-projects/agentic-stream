# Agentic Stream V0.1 — Evaluation Design

This document is the reason the other five weeks are worth spending. It is
pre-registered: **§5 and §6 must be committed before the first scored run**, and
they must not be edited afterwards. If the results are disappointing and the
threshold looks wrong in hindsight, that is a finding, not a bug in the threshold.

## 1. The question

The architecture bets on one claim:

> Under the same context budget, a structured, versioned Situation is better model
> context than filling that budget with the newest raw evidence.

Everything expensive in this project — the compiler, the bands, the immutable
history, the trace, the exact wakes — exists to produce that context. If raw
evidence in a prompt performs as well, the deterministic plane is unpaid overhead
for this class of problem, and the correct response is to change the product, not
to ship.

V0 never ran that comparison (F-01). It compared a rules engine against a rules
engine plus an LLM, which answers a different and much easier question.

## 2. Arms

Three arms, one model, temperature zero, one provider, one prompt objective, one
output schema, and one fixed input-token ceiling. Only the evidence projection
differs.

| Arm | Evidence supplied | Model call | Purpose |
|---|---|---|---|
| **A — deterministic** | selected state, matched condition branch, fact values | none | the floor: what a rules engine already tells you |
| **B — naive** | newest raw events that fit the shared token ceiling, including non-admitted events with the same sanitization as C | one | the cheap alternative: "just prompt it" |
| **C — situated** | the V0.1 snapshot ([TECHNICAL_DESIGN.md §8.1](TECHNICAL_DESIGN.md)) | one | the product |

Arm A produces a rendered string, not a Decision; it is scored on the same rubric
by the same reviewers.

Arm B is the primary comparator. It must be **generous**, not a straw man:

- the same objective, instruction, and output schema as arm C;
- the same input-token ceiling and a realized token count within ±15 % of arm C's,
  verified and reported per scenario;
- events include their timestamps, units, and IDs, so evidence grounding is
  possible;
- non-admitted evidence is included with the same rejection metadata and raw-text
  sanitization as arm C;
- the same prior accepted Decisions are supplied to B and C;
- the entity's operating mode and the model's own threshold constants are stated,
  so the model is not asked to guess the domain;
- the same retry and validation policy.

If arm B is strangled, the result is worthless. Build it to win.

Arm C may summarize more historical samples than arm B can fit verbatim. That is
not a confound; bounded compression is the product claim. It may not receive a
source category unavailable to B: prior Decisions, non-admitted evidence, model
constants, and operating context are either present in both or absent from both.
The comparison is therefore structured compression versus recency, under a common
context budget.

## 3. Ground truth

Hand-written scenarios scored by their own author measure the author (F-02).
Scenarios are generated instead.

`agentic-stream eval generate --seed <n>` produces JSONL traces from a fault
model. Each scenario carries a hidden label and a fault-onset time.

| Label | Generated signature |
|---|---|
| `bearing_wear` | slow vibration RMS ramp over hours, mild temperature follow, normal mode |
| `misalignment` | step change in vibration after a mode change, stable temperature |
| `overload` | temperature ramp correlated with `high_load` mode, vibration within band |
| `lubrication_due` | gradual vibration and temperature drift with `maintenance_age` beyond 30 days |
| `sensor_drift` | one channel drifts monotonically while correlated channels stay flat |
| `sensor_failure` | one channel stops or emits out-of-contract values while the independent heartbeat continues |
| `transient_none` | a spike or burst that resolves without intervention — the false-positive trap |

`sensor_drift`, `sensor_failure`, and `transient_none` exist because they are the
labels a threshold rule *cannot* get right. They are where an LLM should earn its
place. `bearing_wear` and `overload` are where the rule already wins and the model
can only add or destroy value.

Each scenario declares `expected_episode`. It is true for the six diagnostic fault
labels and false for `transient_none`. A missing episode on an expected diagnostic
scenario is a mechanism miss and produces no model output; it is not silently
removed from the result.

Each scenario is additionally perturbed, sampled from a fixed seed:

- ordered / bounded out-of-order / late-beyond-allowance arrivals;
- duplicate bursts;
- a silence gap;
- an out-of-enum operating mode;
- a preceding maintenance event, present or absent.

Minimum suite: **70 scenarios, ≥ 10 per label**, committed as JSONL with a
committed `labels.jsonl` and the generator seed. The suite is regenerable
byte-for-byte from the seed.

Arms B and C run three independent repetitions per scenario even at temperature
zero. Provider inference is not assumed deterministic. Automatic rate metrics use
all repetitions with confidence intervals clustered by scenario; human scoring
uses one repetition selected by the committed run seed so reviewer workload does
not triple.

### 3.1 Honest limitation

Simulator ground truth measures reasoning over a synthetic process, not over a
real motor. It cannot tell you the runtime works in a plant. It can tell you
whether structure beats raw evidence as context, which is the architectural
question — and it can do so with enough scenarios to have a defensible n. State
this limitation in the report; do not let a good simulator score be reported as a
field result.

## 4. Scoring

### 4.1 Automatic, computed for every arm and scenario

| Metric | Definition |
|---|---|
| `label_correct` | exact equality between the hidden label and the configured output value at the suite manifest's `label_pointer` |
| `evidence_valid` | fraction of cited event IDs that exist in the trace |
| `evidence_relevant` | fraction of cited event IDs inside the active window of the triggering Situation |
| `evidence_hallucinated_rate` | nonexistent cited IDs ÷ all cited IDs; zero citations is reported separately |
| `schema_valid` | output parsed and validated on the first attempt; not applicable to A |
| `confidence` | extracted via `confidence_pointer` |
| `overconfidence` | mean `confidence` over scenarios where `label_correct` is false |
| `urgency_mae` | mean absolute ordinal distance from the label's reference urgency |
| `urgency_bias` | signed ordinal distance, reported separately so over- and under-reaction cannot cancel invisibly |
| `input_tokens`, `output_tokens`, `cost_usd` | per call |
| `latency_ms` | per call |

`evidence_hallucinated_rate` and `overconfidence` are scored as **harms**, not as
missing points. A confidently wrong diagnosis is worse than no diagnosis, and an
evaluation that averages them away is lying.

Arm A renders a fixed state/branch-to-label projection committed in the suite
manifest. It includes the supporting event IDs already present in the deterministic
trace. Model-only metrics are `N/A`, not zero; cost and token use are zero.

The suite manifest declares `label_pointer`, `confidence_pointer`, and an ordered
urgency mapping. For the motor fixture the label pointer is `/fault_class`. The
harness contains no motor label names or output paths in code.

### 4.2 Human, blinded

Two reviewers, at least one with domain familiarity, score every scenario's three
outputs. The harness presents them with:

- arm identity removed;
- output order shuffled per scenario from the run seed;
- the scenario's raw trace available on request, but the hidden label withheld
  until scoring is submitted.

Rubric, 0–3 each:

| Dimension | 0 | 3 |
|---|---|---|
| Correctness | contradicts the evidence | identifies the fault mechanism |
| Discrimination | ignores the sensor-fault possibility | correctly separates real fault from sensor fault |
| Grounding | cites nothing, or cites wrongly | every claim traceable to cited evidence |
| Next step | generic or absent | specific, proportionate, actionable |
| Restraint | overstates certainty | states uncertainty and what would resolve it |

Inter-rater agreement (quadratic-weighted Cohen's κ per ordinal dimension) is
reported. κ below 0.4 excludes that dimension from comparative interpretation and
is reported as such, rather than quietly averaged in. The mechanical verdict in
§5 uses automatic metrics and is unaffected by reviewer disagreement.

### 4.3 System metrics, reported alongside

- cognition ratio over the whole suite;
- episodes suppressed, by reason;
- Situation versions per scenario;
- cost per resolved Situation, per arm;
- deterministic-replay hash stability across three runs.

## 5. Pre-registered decision rule

Let `ΔCB = correct-label rate(C) − correct-label rate(B)` over scenarios with
`expected_episode = true` and three model repetitions. A missing expected episode
is incorrect for both arms. Let `ΔCB_hard` be the same over the two ambiguous
diagnostic labels (`sensor_drift`, `sensor_failure`). `transient_none` tests the
deterministic admission plane instead: `transient_episode_rate` is the fraction of
those scenarios that incorrectly admit any episode.

Apply these rules in order:

1. **Stop and redesign** if `ΔCB < 5`, `ΔCB_hard < 5`,
   `overconfidence(C) > overconfidence(B) + 10 points`,
   `evidence_hallucinated_rate(C) > 5 %`, or
   `correct-label rate(C) ≤ correct-label rate(A) + 5`.
2. **Go** if `ΔCB ≥ 15`, `ΔCB_hard ≥ 15`, both scenario-clustered 95 % confidence
   intervals have a lower bound above zero,
   `overconfidence(C) ≤ overconfidence(B)`,
   `evidence_hallucinated_rate(C) ≤ 2 %`, `transient_episode_rate ≤ 5 %`, and
   cognition ratio ≤ 0.5 %.
3. **Investigate** otherwise.

The ordering makes the rule exhaustive and non-overlapping. These numbers are
chosen now, with no results in hand, and are binding.

## 6. Reporting

The evaluation report is a release artifact and contains:

1. suite generator seed, model ID, provider, temperature, prompt digests, model
   digest, and binary version — everything needed to rerun it;
2. the three-arm table of automatic metrics with 95 % bootstrap confidence
   intervals on the rate differences;
3. per-label breakdown, with the two ambiguous diagnostic labels and the
   `transient_none` admission result called out;
4. human rubric means, per dimension and per arm, with κ;
5. cost and latency per arm;
6. every scenario where arm B beat arm C, quoted in full — the disconfirming cases
   are the most informative part of the report and must not be summarized away;
7. the verdict under §5, computed mechanically;
8. the §3.1 limitation, restated.

## 7. What each verdict means next

**Go.** Structure earns its cost. Proceed to the evolution seam in
[TECHNICAL_DESIGN.md §16](TECHNICAL_DESIGN.md): a third domain, then an action
plane. Rerun this suite as a regression gate on every model-version change.

**Investigate.** The signal is real but small. In order: (1) enrich evidence —
prior Decisions and outcomes, longer history, a second window resolution; (2)
check whether arm C's advantage is concentrated in the perturbed scenarios (late,
duplicate, silent), which would mean the value is *temporal correctness* rather
than *structure*, and that is a different and narrower product; (3) rerun with the
enriched snapshot. Do not add features while the verdict is Investigate.

**Stop and redesign.** Do not add orchestration; more scaffolding will not fix a
model that adds nothing. The live options are:

- **Wrong domain.** Motor telemetry may be too thin for cognition to matter
  (F-03). Retest in a domain with heterogeneous evidence — incident response with
  logs and deploys, or account risk with transactions and text.
- **Wrong unit.** If arm A is competitive with both model arms, the useful product
  is the deterministic plane on its own: a versioned, replayable, explainable rules
  runtime with no LLM. That is a smaller and more honest product, and the V0.1
  codebase already is one.
- **Wrong premise.** If arm B matches arm C, then models reconstruct temporal
  truth from raw evidence well enough that owning the Situation-to-cognition
  boundary is not a product. Publish the negative result; it is genuinely useful
  to the category, and it is cheaper to publish it now than after a year of
  building the action plane.

A negative verdict delivered on schedule with a defensible method is a successful
V0.1. That framing has to be established before the run, not after it.
