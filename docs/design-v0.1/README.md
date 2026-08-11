# Agentic Stream V0.1

Status: ready to implement — supersedes [V0](../design-v0/)

Decision date: 2026-07-29

Time box: 25 build days plus a separate 5-day evaluation phase, one senior engineer

## Executive decision

V0.1 is a **single-purpose streaming diagnosis runtime with a real experiment
attached**, not an agent platform.

It accepts events, evaluates an injected and versioned Situation Model, maintains
one durable Situation per entity, and asks one model for a structured diagnosis
only when the model decides that Situation deserves cognition.

```text
JSONL replay or HTTP events
        |
        v
validate + admit + persist (all evidence kept, admitted or not)
        |
        v
injected Situation Model  ->  facts with quality  ->  bands
        |
        v
normal / watch / warning Situation  (hysteresis + sustained duration)
        |
        v
trigger admission  (transition or sustained, with cooldown and budget)
        |
        v
one bounded model call on a frozen snapshot
        |
        v
stored, schema-validated, evidence-checked diagnosis
```

The runtime never executes the recommendation. A human reads it through the CLI
or the API.

## What changed from V0 and why

V0's scoping was right. Its instrument was not. The full assessment is in
[CRITIQUE_OF_V0.md](CRITIQUE_OF_V0.md); the twenty-four findings resolve into five
changes that matter.

### 1. The experiment became the deliverable

V0 spent 25 days on mechanism and one day on measurement, and the measurement it
defined compared "rules engine" against "rules engine plus LLM." It never ran the
comparison the architecture actually depends on: **is a structured Situation
better model context than the raw evidence it was computed from?**

V0.1 runs three arms — deterministic-only, raw-window-dump, and situation-grounded
— against simulator-generated ground truth, blinded, with a decision rule written
before the first run. See [EVALUATION_DESIGN.md](EVALUATION_DESIGN.md). If the
raw-window arm ties, the project has learned something worth more than a shipped
binary.

### 2. `absent` became `age`, and the runtime stopped polling

V0 evaluated absence with a 10-second wall-clock scanner, which made live Situation
history unreproducible and therefore unreplayable — the one guarantee the product
sells. V0.1 replaces the `absent` boolean with an `age` number fact and computes
the exact instant at which any age fact crosses a literal referenced by a
condition. The runtime wakes at that instant instead of polling.

This is strictly more expressive (`days since last maintenance` is now a fact),
strictly more efficient (no scan), and it closes the determinism hole at its root:
the **evaluation instant is always a recorded timestamp, never `time.Now()`**.
Paired with `trace export`, an incident captured live replays to identical
Situation hashes.

### 3. Noise dampening became real

V0 could enter `warning` from a single noisy sample and then hold that state for a
full window, because `max` over a window is a noise amplifier and hysteresis only
prevents the *second* spurious episode. V0.1 adds `sustained_for` on state entry,
`cooldown` on triggers, `min` and `count` facts for sample support, and a compiler
proof that `enter ⇒ stay` so a model cannot be written that flaps by construction.

### 4. Under-specified things became specified

Bands (which control the entire audit trail's granularity), cold-start previous
state, model activation against live entities, window capacity, and injected
output-schema safety were all vague or missing. Each is now exact, and the band
definition carries a provable property: a band changes if and only if some
comparison against that fact could have changed truth value.

### 5. Sequencing moved risk to the front

The conformance proof for the injection invariant moved from Day 21 to Week 1. The
value experiment moved ahead of hardening. The build weeks carry an explicit
ordered cut list so that when the schedule slips — it will — the evaluation is not
what disappears.

## Injection invariant

Unchanged from V0, and now proved in Week 1 rather than Week 5.

The binary contains no domain behavior. Event names, units, enumerations, lateness,
windows, fact definitions, state names, thresholds, hysteresis, sustained
durations, cognition triggers, cooldowns, budgets, episode instructions, and
Decision schemas all come from an injected document loaded through the same public
path as any other domain.

The compiled runtime contains only bounded execution semantics — "compute an
average," "evaluate a typed comparison," "select a prioritized state," "wake when
an age crosses a literal." Those are platform mechanics, not product rules.

Two unrelated models ([motor](examples/motor-warning.situation-model.json),
[cold room](examples/coldroom-integrity.situation-model.json)) run in CI against
one unchanged binary and one unchanged schema from Day 5 onward. Any code branch,
database column, or API path added for one domain fails that test.

## Product hypothesis

The product is worth continuing only if this holds:

> Continuous evidence can be reduced into a stable, versioned Situation such that a
> model is called rarely, receives better context than the raw evidence window, and
> produces a materially more useful diagnosis than either the deterministic rule or
> the raw window alone.

Three clauses, three arms, one pre-registered decision rule. V0 tested one clause
informally. V0.1 tests all three.

## Technology

| Area | V0.1 choice |
|---|---|
| Language | Go 1.26 |
| Process | One binary, one process |
| Persistence | SQLite WAL via `database/sql` and `modernc.org/sqlite` |
| HTTP | Go `net/http` |
| CLI | Go standard `flag` package |
| Logging | Go `log/slog` |
| Canonical JSON | RFC 8785 |
| Schema validation | `santhosh-tekuri/jsonschema/v6`, remote resolution disabled |
| Reasoning | One configured LLM JSON/HTTP provider adapter |
| Test stack | Go `testing`, `httptest`, deterministic fake model |
| Domain model | Injected strict JSON, validated and compiled to typed internal form |
| Input | JSONL replay and loopback HTTP |
| Evaluation | Fault-injecting trace generator, three-arm harness, blinded scoring |

There is no Python runtime, agent framework, broker, RPC layer, ORM, arbitrary
expression language, plugin system, or web application.

## Documents

- [Assessment of V0](CRITIQUE_OF_V0.md) — the twenty-four findings this release
  responds to, and a critique of the project idea itself.
- [V0.1 design review](REVIEW.md) — the implementation blockers found in the
  cross-contract review and how they were resolved.
- [Technical design](TECHNICAL_DESIGN.md) — runtime behavior, contracts, time
  semantics, storage, APIs, failure semantics.
- [Situation Model design](SITUATION_MODEL_DESIGN.md) — the injected domain model,
  operators, bands, hysteresis, triggers, compilation, versioning.
- [Evaluation design](EVALUATION_DESIGN.md) — the three-arm experiment, ground
  truth, scoring, and the pre-registered decision rule.
- [Implementation plan](IMPLEMENTATION_PLAN.md) — build order, gates, cut list,
  stop/go.
- [Situation Model schema](contracts/situation-model-v0.1.schema.json)
- [Event envelope schema](contracts/event-envelope-v0.1.schema.json)
- [Replay trace-record schema](contracts/trace-record-v0.1.schema.json) — events
  plus model activations
- [SQLite schema](contracts/storage-schema-v0.1.sql) — seven tables
- [Motor example](examples/motor-warning.situation-model.json)
- [Cold room example](examples/coldroom-integrity.situation-model.json) — the
  conformance model, a Week-1 fixture
- [Hysteresis reference implementation](reference/hysteresis_reference.py) — an
  executable oracle for the band construction and the `enter ⇒ stay` proof, used
  to verify both example models before writing a line of Go

Both example models validate against the schema, apply cleanly to SQLite, and pass
the hysteresis proof — checked, not assumed:

```bash
python3 reference/hysteresis_reference.py examples/*.situation-model.json
python3 -m unittest discover -s reference -p 'test_*.py'
(cd reference && go test ./...)
```

The broader [version 1 design](../design/README.md) remains research and future
architecture input. It is not an implementation checklist.

Terminology:

- **Situation Model** — the injected deterministic domain configuration.
- **Reasoning model** — the optional LLM used after a trigger is admitted.
- **Fact quality** — `ok`, `partial`, or `unavailable`.
- **Band** — the interval or point of a fact's value within which no comparison
  against it can change truth value.
- **Evaluation instant** — the recorded timestamp at which the model runtime ran.
  Always an event's arrival time or a computed wake time. Never a wall clock read.

## V0.1 boundaries

### Included

- two configured domains, one of which exists only to prove domain independence;
- configurable inputs with mandatory enumeration for string values;
- one explicit configurable lateness allowance per model;
- configurable trailing windows with mandatory sample caps and truncation quality;
- `avg`, `max`, `min`, `count`, `latest`, `age` facts;
- typed condition trees with a closed operator set;
- prioritized states with entry conditions, stay conditions, and sustained
  durations;
- transition and sustained cognition triggers with cooldowns and budgets;
- duplicate suppression by event ID with payload-hash conflict detection;
- non-admitted evidence retained and exposed; raw rejected strings remain in
  storage but are represented by metadata at the reasoning boundary;
- one active, immutable Situation Model version per entity type, with a
  compatibility verdict on activation;
- deterministic replay from JSONL and `trace export` from a live database;
- one bounded, structured model call per admitted trigger;
- stored diagnoses validated against the injected schema with evidence checking;
- loopback HTTP inspection, `explain`, `watch`, `models diff`, `models compare`;
- restart recovery for pending episodes;
- a three-arm evaluation harness with generated ground truth.

### Explicitly excluded

- external actions, tools, approvals, and effectors;
- arbitrary user-defined operators, expressions, or executable scripts;
- LangChain, LangGraph, Hermes, or OpenClaw integration;
- chat, channels, memory, skills, multi-agent delegation, and tool loops;
- MQTT, Kafka, NATS, gRPC, Python workers, and distributed processing;
- multiple model providers or automatic model routing;
- unit conversion — units must match the declaration exactly;
- sliding, session, count, or calendar windows;
- percentile and statistical operators;
- multitenancy and hostile tenant isolation;
- counterfactual replay and a web UI;
- OpenTelemetry, Kubernetes, and production cluster deployment;
- a third production domain.

An excluded item requires a new product decision. It must not enter V0.1 as "small
infrastructure."

## Definition of success

V0.1 succeeds when all of the following hold.

**Mechanism**

1. The same trace produces the same canonical Situation hashes across three runs.
2. A trace ingested live, exported, and replayed produces identical hashes.
3. Duplicates, ID reuse, bounded out-of-order events, and late events cannot
   corrupt a result, and none of them are silently discarded.
4. Noise around a threshold does not produce a spurious episode, and the golden
   traces hold the cognition ratio at or below 0.5 %.
5. One admitted trigger produces one evidence-linked diagnosis, and a restart at
   any transaction boundary can neither lose nor duplicate it.
6. Two unrelated Situation Models run on one unchanged binary and schema.
7. An operator can reconstruct the timeline and the reason for every state change
   and every suppressed trigger from `explain` alone.
8. A 24-hour soak stays within the stated resource bounds.

**Product**

9. The three-arm evaluation completes on ≥ 70 labeled scenarios with blinded
   scoring, and the pre-registered decision rule returns a verdict.

Success is not "the software ran." It is a defensible answer to whether a
versioned Situation earns its cost as model context. A negative answer delivered
on time is a successful V0.1.
