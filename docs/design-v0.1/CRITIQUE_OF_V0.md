# Assessment of V0

Reviewed: [`design-v0/`](../design-v0/) as of 2026-07-29.
Reviewer position: adversarial. The question is not "is this a nice design," it is
"will building exactly this answer the product question, and is it internally
sound enough to build without a rewrite."

## 1. Verdict

V0 is a **good cut with a broken instrument**.

The scoping judgment is right: strip the action plane, strip the framework
adapters, strip the broker, keep event-time correctness and one bounded model
call. The injection invariant is a genuinely strong architectural constraint and
it is testable. The determinism discipline (canonical hashes, golden replay,
model digest bound to every record) is the correct foundation.

But V0 spends 25 days building the *mechanism* and roughly one day measuring the
*hypothesis*, and the measurement it does define cannot distinguish the
project's central claim from its cheapest alternative. Alongside that there are
two correctness holes (live/replay divergence, model activation against live
entities), one under-specified core algorithm (material-change bands), one
missing dampener that the reference research explicitly names (sustained-duration
conditions), and one concrete security hole (unconstrained injected JSON Schema).

Fixing these does not require abandoning V0. It requires re-sequencing it and
tightening about a dozen definitions. That is what
[v0.1](README.md) does.

## 2. What V0 gets right and v0.1 keeps unchanged

| Kept | Why |
|---|---|
| Situation as the unit, not the message | Correct product abstraction; the whole thesis rests on it |
| No action plane in the first release | Removes the hardest safety surface before value is proven |
| Injection invariant — no domain behavior in the binary | Testable, falsifiable, prevents a demo tuned by hardcoding |
| Closed typed operator set instead of CEL/JS/Python | Right tradeoff at this size; static checking and traces are worth the authoring pain |
| One process, SQLite WAL, one writer, no broker | The bottleneck is semantics, not throughput |
| Immutable model versions identified by digest | Makes every stored record interpretable years later |
| Crash injection at every transaction boundary | The only honest way to claim atomicity |
| Deterministic fake model in tests | Correct separation of "the runtime is deterministic" from "the model is not" |
| Stop/go framed as a product question | Rare and healthy; most V0 plans ask "did it run" |

## 3. Findings

Severity: **B**locker (v0.1 must change it), **M**ajor (should change), **m**inor
(note and decide).

### Product and evaluation

#### F-01 (B) — The stop/go experiment does not test the stated hypothesis

The README hypothesis is that a reduced Situation produces "a more useful
diagnosis than a threshold alert alone." The stop/go procedure
([IMPLEMENTATION_PLAN.md §11](../design-v0/IMPLEMENTATION_PLAN.md)) compares
*deterministic Situation only* against *Situation plus model*. That measures
whether an LLM adds anything to a rules engine. It does not measure the thing
the architecture is actually betting on.

The load-bearing claim of this project is: **a structured, versioned Situation is
better model context than the raw evidence window it was computed from.** The
cheap alternative a skeptic proposes — take the last twenty raw events, put them
in the prompt, ask the same question — is never run. If that baseline ties, the
entire Situation apparatus is unpaid overhead for this use case, and V0 would
have shipped without noticing.

*v0.1: three-arm evaluation with the naive raw-window arm as the primary
comparator. See [EVALUATION_DESIGN.md](EVALUATION_DESIGN.md).*

#### F-02 (B) — The evaluation method cannot produce evidence

"20 labeled warning scenarios" scored by "a domain reviewer," where the scenarios
and the Situation Model are authored by the same person who wants the project to
continue, with no blinding, no ground truth, no pre-registered threshold, and no
hallucination metric. This will confirm whatever the author already believes.

*v0.1: ground truth from a fault-injecting simulator, ≥70 scenarios across 7
labels, arm identity hidden and order shuffled, automatic evidence-grounding and
overconfidence metrics, decision rule written down before the first run.*

#### F-03 (M) — The chosen domain gives the model almost nothing to do

A five-minute window over four homogeneous numeric signals is a domain where the
deterministic rule already states the conclusion. The LLM's only possible
contributions are alternative hypotheses, real-fault-versus-sensor-fault
discrimination, and next-step advice — and all three need evidence V0 removed:
maintenance history, prior incidents, prior conclusions and whether they were
right. The V1 design's example includes maintenance records; V0 stripped them and
with them the model's job.

This is the most likely reason for V0 to produce a false negative: the loop could
be sound and still look worthless because the evidence is too thin to reason over.

*v0.1: adds one categorical history input (maintenance events), an `age` fact so
"time since last service" is expressible, and prior accepted Decisions in the
snapshot. Cost: about thirty lines of configuration and one new fact operator.*

#### F-04 (M) — 25 days is roughly half the real cost, and the plan has no cut list

A validating compiler with eleven passes, a typed evaluator with full traces, an
event-time engine, migrations, six crash-injection points, an HTTP API, a
provider adapter, a durable worker, ten golden traces, fuzzing, a 24-hour soak, a
second domain model, a product evaluation, and a runbook is not 25 days. Nothing
in the plan says what to cut under pressure — so the thing that gets cut is Day
24, the evaluation, which is the only day that answers the product question.

*v0.1: 25 build days plus a separate, non-compressible 5-day evaluation phase,
with an explicit ordered cut list for the build weeks.*

#### F-05 (M) — The injection invariant is proven in week 5, three weeks too late

The second synthetic model is the only proof of the project's headline
architectural constraint, and it is scheduled at Day 21. Discovering motor-shaped
assumptions in the runtime on Day 21 is a rewrite, not a fix. Worse, a second
model authored last, by the same person, against a finished runtime, will
unconsciously be shaped to fit it.

*v0.1: the second model is a Week-1 fixture. Both models run in CI from Day 5.*

### Correctness

#### F-06 (B) — Live history cannot be replayed, so the replay guarantee is void in production

Determinism is asserted for "an ordered JSONL file." Live ingestion is different
in two ways that break it:

1. `absent` facts are evaluated by a 10-second polling scanner against the wall
   clock. The instant at which an absence becomes material depends on the scan
   phase, which is not a function of the input.
2. `arrival_time` is assigned by the server in live mode and there is no command
   that exports accepted events back to a replayable trace. Even if you wanted
   to reproduce an incident, you cannot get the input out.

"Replay before autonomy" is a stated principle of the reference research. V0
fails it for exactly the data that matters — real incidents.

*v0.1: `absent` is replaced by an `age` fact whose literal crossings are computed
exactly, so the runtime wakes at the crossing instead of polling; the evaluation
instant is always a recorded timestamp, never `time.Now()`; `trace export`
regenerates a replayable JSONL from the database; and a conformance test asserts
ingest → export → replay produces byte-identical Situation hashes.*

#### F-07 (B) — Material-change detection is under-specified, and it controls the entire audit trail

TECHNICAL_DESIGN §7 says a new Situation version is written when "a fact crosses
a literal boundary referenced by a condition." `entity_state` stores
`fact_bands_json`. Nothing defines:

- what a band is for a string, a boolean, or an unavailable fact;
- whether the literal set is per-fact-global or per-state;
- how `eq`/`neq` against a number produces a band (a point, not an interval);
- whether `unavailable → available` is material.

The last one is not academic: it is the difference between "no data" and "data
below the threshold," and a diagnostician needs to see it.

*v0.1: bands are a compiler output with an exact definition and a provable
property — a band change occurs if and only if some comparison against that fact
could have changed truth value. Point literals get their own singleton bands, so
`gt` and `gte` are distinguished. `unavailable` is a distinguished band.*

#### F-08 (M) — Activating a new model version against live entities is undefined

Activation "affects future processing" and does not rewrite existing Situations.
So a live entity's `entity_state` row keeps a `current_state` that may not exist
in the new model, `facts_json` keys that the new model never populates, and the
next Situation records `previous_state` from a different model. Trigger `from`
matching across a model change has no defined behavior.

*v0.1: the compiler emits a compatibility verdict against the currently active
version (`compatible` / `reset_required`). `models activate` requires an explicit
mode, refuses `--mode=continue` on an incompatible pair, and always writes a
`model_migration` Situation version per live entity recording both digests.*

#### F-09 (M) — The state algorithm contains a dead branch and no well-formedness check

The algorithm re-evaluates *all* states' `enter` conditions after the current
state's `stay` fails, including the current state's own `enter`. Since `stay` is
supposed to be weaker than `enter`, a failed `stay` implies a failed `enter` — so
that branch is dead in well-formed models. It is not dead in ill-formed ones: a
model where `stay` is *stricter* than `enter` will flap on every evaluation,
which is precisely the failure hysteresis exists to prevent, and nothing rejects
it.

With a closed operator set over a finite literal set, `enter ⇒ stay` is
statically decidable.

*v0.1: the compiler proves `enter ⇒ stay` by exhaustive band enumeration and
rejects models that fail it; the redundant branch is removed.*

#### F-10 (M) — No sustained-duration condition, and `max` over a window is the worst possible noise amplifier

The reference research names four dampeners: debounce, hysteresis, cooldown, and
minimum duration. V0 has one — condition-based hysteresis. The motor example
enters `warning` on `vibration_max >= 7.0`, so a **single** noisy sample commits
the transition, fires an episode, and then pins the fact above the `stay`
threshold of 6.0 for the full five-minute window. Gate A claims "noise around a
threshold does not repeatedly call the model"; hysteresis prevents the *second*
call, not the first spurious one.

*v0.1: adds `sustained_for` on state entry (candidate must hold continuously
across evaluations for a duration in event-horizon time), a `cooldown` on
triggers, and `min`/`count` facts so a model can require sample support. The
motor example is rewritten to require sustained evidence.*

#### F-11 (M) — Transition-only triggers create two dead zones

`cognition_triggers` match `from → to` only. Consequences never stated:

- **Cold start.** An entity whose first event is already a warning has no
  previous state. If previous state is "none," `from: ["normal","watch"]` never
  matches and the entity is never diagnosed.
- **Staleness.** An entity that sits in `warning` for six hours while conditions
  materially worsen receives exactly one diagnosis, produced from the first
  minute of evidence, forever.

*v0.1: cold-start previous state is defined as the model's default state and
tested; a `sustained` trigger type admits at most one episode per `min_interval`
while an entity remains in a state.*

#### F-12 (M) — Full fact recomputation per event and 24-hour windows are mutually incoherent

Recomputing from persisted rows on every event is the right simplicity tradeoff.
But the compiler permits 24-hour windows while the capacity target is 100
events/second, and nothing bounds the product. A 24-hour window on a
10 Hz input is 864,000 rows scanned per event, per fact. The design's stated
targets are met only by accident of the motor example's numbers.

*v0.1: every window declares `max_samples`; the compiler enforces it; the runtime
retains the newest N and marks the fact `partial` when it truncates. Partial
quality reaches the trace, the Situation, and the model — which also implements
the research principle that completeness belongs in the Situation contract.*

#### F-13 (m) — Duplicate detection ignores the payload hash it stores

`events.payload_hash` is persisted and never compared. An ID reused with
different content is silently treated as a duplicate. That is a broken producer,
and it should be loud.

*v0.1: hash mismatch on a known ID returns `409 event_id_reuse`, increments a
distinct counter, and logs the entity — never silently accepted.*

#### F-14 (m) — Rejected-but-well-formed events vanish

A sensor that starts emitting out-of-range or out-of-enum values is one of the
most diagnostically important things that can happen, and V0 turns it into a
counter. Late events are persisted with `late_ignored`; value-level rejections
are not persisted at all.

*v0.1: `late_ignored` generalizes to `admitted` + `admission_reason`. Well-formed
envelopes with unusable values are persisted as non-admitted evidence and appear
in the snapshot. Malformed envelopes are still rejected before persistence.*

#### F-15 (m) — Late-ignored evidence is hidden from the model

Aggregates correctly exclude late events. But the snapshot's twenty recent events
also exclude them, so the model never learns that contradicting evidence arrived
too late to count. That is exactly what a diagnostician wants to know.

*v0.1: the snapshot includes non-admitted events, explicitly flagged.*

### Security

#### F-16 (B) — Injected output schemas are unvalidated JSON Schema

`episodeType.output_schema` is constrained only to `{"type":"object",
"minProperties":1}`. Anything else is accepted and handed to a JSON Schema
compiler. That permits `$ref` to a remote URI, which turns "install a model
document" into a network fetch performed by the runtime at compile time, and it
permits arbitrarily large or deeply nested schemas.

*v0.1: injected schemas are restricted to a keyword allowlist with no `$ref`,
`$id`, `$defs`, or combinators; the root must be a closed object; node count and
depth are capped; the validator is configured with remote resolution disabled.
Go's RE2 makes `pattern` safe from catastrophic backtracking — state it rather
than assume it.*

#### F-17 (M) — The untrusted-content boundary is a sentence in a prompt

§9.1 instructs the model that event content is data, not instruction. That is
not a control. Free-text string values flow from ingress into the snapshot into
the prompt, and the only barrier is the model's willingness to obey.

The cheap structural fix is capability subtraction at the schema, which is the
pattern the reference studies already recommend: enumerate the legal values.
`operating_mode` is obviously enumerable. So is a maintenance code. So is nearly
every string a v0-class model needs.

*v0.1: `enum` is **required** for every string input. Out-of-enum values never
reach the model. The free-text ingress-to-prompt path does not exist in v0.1.*

#### F-18 (m) — `/debug/vars` default-on, auth unstated

`expvar` publishes the process command line and memory statistics. The design
allows non-loopback binding behind a bearer token but never says whether
`/debug/vars` is behind it.

*v0.1: `expvar` is off unless `--debug-vars` is passed, and every non-health route
requires the token when bound off-loopback.*

### Operations and observability

#### F-19 (M) — "Call the model rarely" is the thesis and is neither measured nor capped

There is no episodes-per-hour ceiling, no per-entity ceiling, no cost accounting,
and no metric for the headline number the reference research asks for: the
fraction of events absorbed without cognition. A trigger misconfiguration can
bill an unbounded number of model calls and nothing notices.

*v0.1: hard per-entity and process-wide episode budgets with durable
`suppressed_budget` records; cognition ratio is a first-class reported metric and
an acceptance gate; token counts and estimated cost are stored per episode.*

#### F-20 (M) — Gate D depends on CLI output that is designed nowhere

"An operator can understand the timeline from CLI output alone" is a release
gate. The evaluation trace exists in the database; no command renders it. `explain`
appears in the V1 CLI and was dropped from V0.

*v0.1: `explain` and `watch` are specified commands with defined output, built in
Week 2, not polished in Week 4.*

#### F-21 (M) — Model comparison is a one-day task with no contract

Day 20 says replay two versions into isolated databases and compare. There is no
`compare` command, no diff contract, and no defined notion of divergence. This is
one of the most valuable capabilities in the whole design and it is a sentence.

*v0.1: `models diff` (static) and `models compare` (replay two versions over one
trace) with a defined divergence report: first divergent Situation version,
per-state occupancy delta, trigger admission delta, episode count delta.*

#### F-22 (m) — `readyz` contradicts the failure table

`/readyz` requires "worker running." The failure table says a model outage must
not stop event processing. If the provider is down, is the runtime unready — and
therefore drained by a load balancer — while it is still correctly ingesting?

*v0.1: readiness covers migrations and the write path only. Cognition health is a
separate reported field, never a readiness failure.*

### Schema and storage

#### F-23 (m) — Storage constraint issues

- `situation_models` carries `PRIMARY KEY (id, version)`, `UNIQUE (digest)`, and
  `UNIQUE (id, version, digest)`. The third is implied by the first. The global
  digest uniqueness also means re-installing byte-identical content under a new
  version number fails with an opaque constraint error rather than a clear
  message.
- `situations` has a foreign key to `entity_state`, a mutable projection. Immutable
  history should not depend on the existence of a mutable row.
- `episodes.attempt_count` is unbounded and `retry-episode` has no cap.

*v0.1: redundant composite constraint removed; digest semantics are explicit and
include ID/version, with the global digest retained as the normalized foreign-key
target; duplicate installation is detected before insert with a stable error; the
inverted dependency is dropped; total attempts are capped.*

#### F-24 (m) — `arrival_time` is unvalidated

Replay accepts `arrival_time` from the file, hand-authored traces will contain
whatever the author typed, and nothing requires `arrival_time >= event_time` or a
monotonic arrival sequence. Both are assumed by the horizon and age logic.

*v0.1: both are validated at ingress in every mode.*

### Correct choices that read as problems and are not

| Observation | Assessment |
|---|---|
| One process-wide mutex around writes | Fine — SQLite has one writer regardless. The doc justifies it as a correctness choice; it is a consequence of the storage engine. Record the real reason. |
| Exact unit matching, no conversion | Correct v0 non-goal. State it explicitly rather than leaving it implicit. |
| No SSE / no streaming API | Correct for this scope. A polling `watch` command is enough. |
| JSON expression trees are unpleasant to author | Accepted deliberately and correctly; authoring format comes after semantics are proven. |
| At-least-once model attempts, not exactly-once billing | Honest and correctly documented. Keep the wording. |

## 4. Critique of the project idea itself

Separate from the plan: is the thing worth building?

**The insight is real.** The gap between continuous temporal state and bounded
cognition is genuinely unowned. Confluent Streaming Agents and StreamNative's
Agent Engine validate the category while preserving the wrong unit of work —
they map topics to agent invocations. "Fewer, better-timed episodes over a
versioned Situation" is a materially different and better-founded position.

**The moat is thinner than the design assumes.** Strip the vocabulary and the
deterministic plane is a rules engine over windows. Prometheus recording rules
plus Alertmanager, Flink CEP, Grafana alerting, and Node-RED all compute
thresholds over time windows today, and any of them can post a webhook to a
script that prompts a model. What those cannot do is: version and hash the
interpretation, replay it, explain which branch fired with evidence references,
and record why cognition was *not* invoked. That list — not the aggregation — is
the product. It should be what the demo leads with and what the evaluation
measures.

**The riskiest assumption is unexamined.** The design assumes structured
Situation context beats raw evidence as model input. That is plausible — it is
also exactly the kind of plausible assumption that fails on contact. It is
cheap to test and expensive to be wrong about, which makes it the first thing to
measure, not the last. F-01 is the most important finding in this document.

**The injection invariant is worth its cost, but not at its current position.**
An honest skeptic says: hardcode the motor model, prove value in five days, then
generalize. That is wrong, because a hardcoded model gets tuned until the demo
looks good and the result is unfalsifiable — and because generalizing afterwards
is a rewrite, not a refactor. Keep the invariant. Move the proof to Week 1 and
move the value experiment ahead of the hardening work, so the expensive weeks are
spent on something already known to be worth hardening.

**Second-order risk: the loop can be sound and still look worthless.** If the
evidence is too thin for a diagnosis to be interesting (F-03), the evaluation
returns a false negative and a correct architecture gets abandoned. The
evaluation design must be able to tell "the Situation idea does not help" apart
from "this domain does not need help," which requires more than one scenario
family and at least one input the deterministic rules cannot resolve on their own.

## 5. Disposition

| Finding | Severity | Disposition in v0.1 |
|---|---|---|
| F-01 evaluation does not test the hypothesis | B | Fixed — three-arm design |
| F-02 evaluation method | B | Fixed — ground truth, blinding, pre-registration |
| F-06 live history unreplayable | B | Fixed — `age` facts, exact wake, `trace export` |
| F-07 bands under-specified | B | Fixed — exact definition with a proved property |
| F-16 unvalidated injected schemas | B | Fixed — keyword allowlist, no remote resolution |
| F-03 thin domain | M | Fixed — maintenance input, `age`, prior Decisions |
| F-04 timeline and no cut list | M | Fixed — 25 + 5 days, ordered cut list |
| F-05 conformance proof too late | M | Fixed — Week 1 fixture |
| F-08 activation vs live entities | M | Fixed — compatibility verdict and migration record |
| F-09 dead branch, no `enter ⇒ stay` | M | Fixed — static check |
| F-10 no sustained duration | M | Fixed — `sustained_for`, `cooldown`, `min`/`count` |
| F-11 cold start and staleness | M | Fixed — defined cold start, `sustained` trigger |
| F-12 window/rate incoherence | M | Fixed — `max_samples` and `partial` quality |
| F-17 prompt-only trust boundary | M | Fixed — mandatory string enums |
| F-19 cognition unmeasured/uncapped | M | Fixed — budgets and cognition ratio gate |
| F-20 no `explain` | M | Fixed — specified command, Week 2 |
| F-21 no comparison contract | M | Fixed — `models diff` / `models compare` |
| F-13, F-14, F-15, F-18, F-22, F-23, F-24 | m | All fixed; individually small |

Nothing in this list requires a new subsystem. The largest single addition is the
evaluation harness, and it is the deliverable that justifies the other five weeks.
