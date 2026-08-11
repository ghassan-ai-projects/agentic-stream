# Agentic Stream V0.1 — Configurable Situation Model

Normative. Where this document and the JSON Schema disagree, this document is the
intent and the schema is the bug.

## 1. Decision

Domain behavior is injected as configuration from the first release. Go code
implements a generic deterministic model runtime. Situation Models configure:

- accepted event inputs, units, and value enumerations;
- event-time windows, lateness, and sample capacity;
- derived facts and their quality semantics;
- Situation states, priority, entry, stay, and sustained duration;
- cognition triggers, cooldowns, and budgets;
- episode objective, instruction, and structured output schema.

Thresholds, durations, state names, event names, condition structure, cognition
transitions, domain prompt text, and Decision shape are never compiled into the
binary.

V0.1 does not accept arbitrary code or a general expression language. The injected
model is a typed JSON document with a closed operator set.

### 1.1 Why a constrained AST

CEL, JavaScript, Python, and user plugins offer flexibility at the cost of type
ambiguity, non-determinism, security exposure, and a much larger debugging
surface. A JSON expression tree is unpleasant to author by hand and has properties
that matter more at this stage:

- strict schema validation;
- no parser ambiguity;
- static fact and type checking;
- complete operator allowlisting;
- deterministic canonical hashing;
- **statically decidable well-formedness** — with a finite literal set and monotone
  comparisons, `enter ⇒ stay` can be proved by the compiler (§6.1);
- **exact band computation**, which is what makes material-change detection
  provably correct (§4);
- evaluation traces for every node;
- no hidden I/O or clock access.

An authoring format can be improved once the semantic model is proven. Semantics
first, syntax second.

## 2. Document structure

Machine contract:
[situation-model-v0.1.schema.json](contracts/situation-model-v0.1.schema.json).
Worked examples: [motor](examples/motor-warning.situation-model.json) and
[cold room](examples/coldroom-integrity.situation-model.json).

```json
{
  "schema_version": "0.1.0",
  "id": "motor-warning",
  "version": "1.0.0",
  "entity_type": "motor",
  "lateness_allowance": "2m",
  "inputs": {},
  "windows": {},
  "facts": {},
  "states": [],
  "episode_types": {},
  "cognition_triggers": [],
  "budgets": {}
}
```

### 2.1 Inputs

An input maps a domain name to an event type and a value contract.

```json
{
  "vibration": {
    "event_type": "motor.vibration",
    "value_type": "number",
    "unit": "mm/s",
    "min": 0.0,
    "max": 200.0
  },
  "operating_mode": {
    "event_type": "motor.operating_mode",
    "value_type": "string",
    "enum": ["normal", "high_load", "maintenance", "idle"]
  },
  "heartbeat": {
    "event_type": "motor.heartbeat",
    "value_type": "none"
  }
}
```

Rules:

| Value type | `unit` | `enum` | `min`/`max` |
|---|---|---|---|
| `number` | required | forbidden | optional, both or neither |
| `string` | forbidden | **required**, 1–64 entries | forbidden |
| `none` | forbidden | forbidden | forbidden |

`enum` is mandatory for strings and is a security control, not ergonomics
(F-17). It means no free text can travel from an untrusted producer into a model
prompt. A value outside the enumeration, or a number outside `[min, max]`, is
well-formed but unusable: the event is persisted as non-admitted evidence with
reason `value_out_of_contract`. A rejected number may appear in the episode
snapshot. A rejected string appears there only as its type, length, and SHA-256
digest; its raw text remains in the event store and never crosses the reasoning
boundary. A unit mismatch uses the same admission reason and the same
metadata-only treatment for the producer-supplied unit. Units must match exactly;
V0.1 performs no conversion.

### 2.2 Windows

V0.1 supports fixed trailing event-time windows only.

```json
{
  "recent": {
    "duration": "5m",
    "max_samples": 2000
  }
}
```

`lateness_allowance` belongs to the Situation Model, not to a window. The runtime
stores one admission verdict per event, so per-window allowances would be false
precision: one event could be timely for one window and late for another while the
database has only one `admitted` bit. A single explicit model-level allowance
keeps persistence and semantics aligned. Independent window allowances would
require per-window admission records and are outside V0.1.

`max_samples` is **required** (F-12). It bounds the per-entity, per-input row set
that any aggregate may scan, which is what keeps recompute-per-event honest and
keeps a 24-hour window from silently becoming an unbounded scan. When a window
holds more admitted samples than `max_samples`, the newest `max_samples` are used
and every fact computed over that window is marked `partial`.

Window membership is exactly
`event_horizon - duration < event_time <= event_horizon`. "Newest" is ordered by
`(event_time, arrival_time, id)` descending; aggregate evaluation then consumes
the retained set in the reverse, ascending order. `avg` uses one deterministic
Neumaier compensated sum in Go rather than SQLite's aggregate function. These
details prevent boundary and floating-order differences from becoming replay
differences.

Durations use a restricted Go-duration subset: a positive integer followed by
`ms`, `s`, `m`, or `h`. Bounds are in §7.

### 2.3 Facts

Six operators. Every fact evaluates to a `(value, quality)` pair.

```text
avg(input, window)    -> number  | unavailable | partial
max(input, window)    -> number  | unavailable | partial
min(input, window)    -> number  | unavailable | partial
count(input, window)  -> number  (never unavailable; 0 when empty) | partial
latest(input)         -> number | string from the greatest admitted
                         (event_time, arrival_time, id) tuple | unavailable
age(input)            -> seconds since the arrival of that same latest-by-event-time
                         input event, measured at evaluation instant | unavailable
```

`count` is the completeness primitive: a model can require `count >= 5` before
trusting an average. `age` replaces V0's `absent` boolean (F-06). It is strictly
more expressive — `heartbeat_age > 120` is the old `absent`, and
`maintenance_age > 2592000` is "no service in thirty days" — and it makes the
timer exact rather than polled (§5).

The tuple tie-break is part of determinism. A bounded out-of-order event with an
older `event_time` is retained and may change a window aggregate, but it cannot
replace `latest` or reset `age`. Otherwise a stale heartbeat arriving late would
falsely make a silent source look healthy.

Facts declare their operator and references; all references must resolve during
compilation. Numeric aggregates accept only numeric inputs. `latest` on a `none`
input is rejected — `none` inputs carry no value and exist only for `age` and
`count`.

### 2.4 Fact quality

| Quality | Meaning | Comparison behavior |
|---|---|---|
| `ok` | computed over a complete window | evaluates normally |
| `partial` | computed, but the window was truncated at `max_samples` | evaluates normally; recorded in the trace, the Situation, and the snapshot |
| `unavailable` | no admitted samples, or the input has never arrived | every comparison is **false**, trace records `fact_unavailable` |

There is no implicit coercion between number, string, and boolean anywhere.
`unavailable` is never silently treated as zero.

### 2.5 Conditions

The closed operator set:

```text
all, any, not
gte, gt, lte, lt, eq, neq
fact, number, string
```

V0's `boolean` operand is gone. With `absent` replaced by the numeric `age` fact,
no fact has boolean type, so the operand was unreachable — and an unreachable
operator in a closed set is a liability, not headroom.

```json
{"gte": [{"fact": "vibration_max"}, {"number": 7.0}]}
```

```json
{
  "all": [
    {"gte": [{"fact": "vibration_avg"}, {"number": 5.0}]},
    {"gte": [{"fact": "vibration_count"}, {"number": 5}]},
    {"gte": [{"fact": "temperature_avg"}, {"number": 80.0}]}
  ]
}
```

Every comparison is **fact on the left, literal on the right**. Comparison typing
is checked at compile time: a numeric fact may only be compared against a number
literal, a string fact only against a string literal declared in that input's
`enum`, and `eq`/`neq` are the only operators legal on strings. The compiler
rejects a string comparison against a literal that the input's enumeration cannot
produce — that is always an authoring error.

An `age` fact may use `gte`, `gt`, `lte`, or `lt`, but not `eq` or `neq`. Equality
against a continuously increasing clock would be true for a single timestamp and
would require a second wake one nanosecond later. Age literals are non-negative
seconds with at most nine fractional digits and compile to exact integer
nanosecond boundaries.

**Fact-to-fact comparison is excluded**, and the reason is not conservatism: it
would break the band property in §4.2. Two facts can each remain inside their own
band while crossing each other, so `{"gte": [{"fact":"a"},{"fact":"b"}]}` would
make material-change detection unsound. Relative comparison is a real capability
gap — "vibration is 50 % above its four-hour baseline" cannot be expressed — and
it is deferred until bands are extended to cover it, not until someone asks.

### 2.6 States, hysteresis, and sustained duration

```json
{
  "id": "warning",
  "priority": 100,
  "enter": {"gte": [{"fact": "vibration_max"}, {"number": 7.0}]},
  "stay":  {"gte": [{"fact": "vibration_max"}, {"number": 6.0}]},
  "sustained_for": "90s"
}
```

Exactly one state carries `"default": true`, has the lowest priority, and has no
conditions. Priorities are unique.

`sustained_for` (F-10) requires a candidate state's `enter` condition to have held
continuously since `evaluation_instant - sustained_for` before the transition
commits. "Continuously" means *at every evaluation performed in that interval* —
the honest, testable definition, since the runtime has no information between
evaluations.

The duration is measured on the **evaluation-instant axis**, not the event
horizon. This matters: during silence the event horizon is frozen by definition,
so a horizon-measured duration could never be satisfied by a silence-driven
condition — a state entered because `heartbeat_age` crossed a threshold would
stay pending forever. The evaluation instant advances on both events and wakes, so
it works for both, and it is equally deterministic because every evaluation
instant is a recorded or exactly computed timestamp.

While a candidate is pending, the entity remains in its current state and the
trace records the candidacy:

```json
{"candidate": "warning", "since_ns": 1753776900000000000, "required": "90s", "held": "45s"}
```

Candidacy is durable (`entity_state.candidacies_json`) and survives restart. It
resets the moment `enter` evaluates false.

#### Evaluation algorithm

Given the current state `S` (for an entity with no prior state, `S` is the model's
default state — see §6.3):

1. Evaluate `enter` for every non-default state other than `S`.
2. For states that declare `sustained_for`, update candidacy timers: start on a
   rising edge, clear on a falling edge. A state without `sustained_for` is
   immediately eligible and creates no synthetic candidacy record. Tracking
   lower-priority candidates matters when the current state's `stay` later becomes
   false.
3. Among higher-priority states whose `enter` is true **and** whose
   `sustained_for` is satisfied, select the highest priority. If one exists, it is
   the selected state.
4. Otherwise, if `S` has a `stay` condition, evaluate it. If true, `S` is retained.
   If `S` has no `stay`, its `enter` is used in its place.
5. Otherwise, consider every lower-priority state in descending priority and
   select the first whose already-updated entry candidacy is satisfied.
6. Otherwise select the default state.

Step 5 in V0 re-evaluated *all* states including the current one, which is dead
code in a well-formed model and a flap in an ill-formed one (F-09). V0.1 restricts
step 5 to lower-priority states and makes ill-formed models impossible by proving
`enter ⇒ stay` at compile time (§6.1).

Leaving the current state is never delayed by that state's `sustained_for`;
leaving is governed by `stay`. A lower target that declares its own
`sustained_for` must still have satisfied that entry requirement. If none has,
selection falls to the default state.

### 2.7 Cognition triggers

Two types. Both require an `episode_type` and a `cooldown`.

```json
{
  "id": "diagnose-warning",
  "type": "transition",
  "from": ["normal", "watch"],
  "to": "warning",
  "episode_type": "diagnosis",
  "cooldown": "30m"
}
```

```json
{
  "id": "review-persistent-warning",
  "type": "sustained",
  "state": "warning",
  "min_interval": "6h",
  "episode_type": "diagnosis",
  "cooldown": "6h"
}
```

`transition` matches when the previous and selected states match `from` and `to`.
`sustained` becomes due while the entity remains in `state` and at least
`min_interval` has passed since that state was entered or since the last episode
admitted by that trigger, whichever is later. Its due time is a wake source (§5),
so it does not depend on an unrelated event creating a Situation version.
`sustained` exists because a transition-only model diagnoses a six-hour warning
exactly once, from its first minute of evidence (F-11).

`cooldown` is shared by all triggers for the same entity and `episode_type`;
`min_interval` is the cadence of one sustained trigger. The effective eligibility
time is the later of those boundaries. The fields are complementary rather than
redundant.

The evaluator emits a durable admission outcome for every trigger that matched.
It never calls the model itself.

| Outcome | Meaning |
|---|---|
| `admitted` | an episode row was created in the same transaction |
| `suppressed_cooldown` | matched, but within the trigger's cooldown |
| `suppressed_budget` | matched, but a per-entity or process budget is exhausted |

Every matched-and-suppressed opportunity is recorded in the Situation's
`triggers_json` with its reason and its release time. Nothing that could have
caused cognition disappears without a durable explanation.

For a transition trigger, suppression is final for that transition. For a
sustained trigger, cooldown or budget suppression schedules one retry at the exact
release time. This distinction avoids both lost long-running reviews and a
zero-delay wake loop.

### 2.8 Budgets

```json
{
  "budgets": {
    "max_episodes_per_entity_per_hour": 4
  }
}
```

The budget is mandatory: there is no hidden domain default. The process
additionally enforces `--max-episodes-per-hour` (range 1–10,000, default 100).
V0.1 deliberately has no suppression rule based on asynchronous episode status:
whether a provider call has finished must not change deterministic trigger
admission. One worker still bounds model-call concurrency.
"Call the model rarely" is the product thesis; V0 neither measured nor capped it
(F-19). V0.1 caps it in configuration, counts it, and gates a release on it.

### 2.9 Episode types

```json
{
  "diagnosis": {
    "objective": "Diagnose the motor condition.",
    "instruction": "Use only supplied evidence and return JSON.",
    "output_schema": { "type": "object", "additionalProperties": false, "...": "..." },
    "evidence_reference_pointer": "/evidence_event_ids",
    "confidence_pointer": "/confidence"
  }
}
```

The runtime adds only generic safety framing, the frozen Situation snapshot, and
the compiled output schema. It validates output against that schema and verifies
that every ID at `evidence_reference_pointer` exists in the persisted snapshot.

`confidence_pointer` is optional and new: when present it must resolve to a number
in `[0, 1]`, and the value is extracted into a first-class column so the evaluation
harness can measure overconfidence without parsing domain JSON.

#### Output-schema restrictions

Injected schemas are a security surface, not free-form configuration (F-16). The
compiler accepts only this keyword set:

```text
type, properties, required, additionalProperties, items, prefixItems,
enum, const, minimum, maximum, exclusiveMinimum, exclusiveMaximum,
minLength, maxLength, pattern, minItems, maxItems, uniqueItems,
minProperties, maxProperties, description
```

and rejects everything else, explicitly including `$ref`, `$dynamicRef`, `$id`,
`$anchor`, `$defs`, `allOf`, `anyOf`, `oneOf`, `not`, `if`/`then`/`else`,
`unevaluated*`, `dependentSchemas`, `propertyNames`, and `contentSchema`.

Additional constraints:

- the root must be `{"type": "object", "additionalProperties": false}`;
- at most 200 schema nodes and 8 levels of nesting;
- the JSON Schema compiler is configured with remote reference resolution
  **disabled**, so a malformed allowlist cannot become a network fetch;
- `pattern` values must compile; Go's RE2 engine is linear-time, so patterns
  cannot cause catastrophic backtracking — this is a property of the chosen
  runtime and is stated rather than assumed;
- `evidence_reference_pointer` must resolve to a declared property of type
  `array` with `items: {"type": "string"}`.

## 3. Time semantics inside the model

Two clocks, both explicit inputs to evaluation. Neither is read from the wall
clock during evaluation.

| Clock | Source | Used by |
|---|---|---|
| Event horizon | maximum admitted `event_time` for the entity | windows |
| Evaluation instant | the triggering event's `arrival_time`, or the exact computed wake time for a timer evaluation | `age`, `sustained_for`, cooldowns, budgets, sustained-trigger cadence |

The **evaluation instant is always a recorded timestamp** (F-06). The runtime uses
the wall clock only to decide *when* to run a timer evaluation, never to compute a
value that enters a Situation. This is the single rule that makes live history
replayable.

## 4. Bands and material change

A **band** is the interval or point within which a fact's value cannot change the
truth of any comparison against it. Bands are a compiler output, and they are the
mechanism that decides when an immutable Situation version is written (F-07).

### 4.1 Definition

For each fact `f`, let `L(f) = [l₁ < l₂ < … < l_k]` be the sorted distinct set of
literals compared against `f` anywhere in any `enter` or `stay` expression in the
model.

For a numeric fact the band domain is:

```text
band(f) = "unavailable"                     if quality = unavailable
        = i0   if value < l₁
        = p1   if value = l₁
        = i1   if l₁ < value < l₂
        = p2   if value = l₂
        …
        = pk   if value = l_k
        = ik   if value > l_k
```

That is, the real line is partitioned into open intervals `i0…ik` separated by
singleton point bands `p1…pk` at each literal — `2k + 1` bands plus
`unavailable`. Point bands exist so `gt` and `gte` are distinguished, which they
must be, since they differ at exactly one value.

Worked example, from the motor model. `vibration_max` is compared against `7.0`
(warning entry) and `6.0` (warning stay), so `L = [6.0, 7.0]` and the band domain
is:

| Band | Values | `>= 7.0` | `>= 6.0` |
|---|---|---|---|
| `i0` | `< 6.0` | false | false |
| `p1` | `= 6.0` | false | **true** |
| `i1` | `6.0 … 7.0` | false | true |
| `p2` | `= 7.0` | **true** | true |
| `i2` | `> 7.0` | true | true |
| `unavailable` | — | false | false |

A reading of 7.2 is band `i2`. A reading of 8.9 is also band `i2` — no Situation
version is written for the change, because no condition in the model can tell them
apart.

For a referenced string fact, `band(f) = "s:" + value` (finite, because inputs
enumerate their values). String-ness is decided from the fact's declared type. For
a `latest` fact over a `none` input, no band exists — such a fact is rejected at
compile time.

`quality = partial` does not change the band but *is* itself material: a fact that
becomes partial has a different evidentiary status even at the same value.

### 4.1.1 Evidence-only facts

A fact that no condition references has `L(f) = ∅` and therefore exactly one
`unreferenced` value band plus `unavailable`, regardless of whether it is numeric
or string. This is legal and useful — `last_maintenance` and `operating_mode` in
the motor example exist purely to give the reasoning model context — but it has a
consequence an implementer will otherwise discover the hard way:

**Changes to an evidence-only fact never create a Situation version.** A motor
whose `last_maintenance` moves from `lubricated` to `bearing_replaced` produces no
new immutable Situation, because no decision could have changed. The value is
still current in `entity_state`, still present in the next snapshot, and still
visible in `explain`; it just does not appear as an event in the Situation
timeline.

That is the correct behavior under §4.2, and it is the intended trade: the
Situation history records decisions, and evidence that cannot affect a decision
belongs in the event log, which already has it. An author who wants a maintenance
change to be visible in the timeline must give it a condition — which is to say,
must make it decision-relevant. The compiler emits an informational note listing
every evidence-only fact so this is a choice rather than a surprise.

### 4.2 The property that makes this correct

**Claim.** If no fact changes band and no fact changes quality class, then no
comparison in the model can change truth value, and therefore neither the selected
state nor any trigger can change.

*Reason.* Every comparison against `f` uses a literal in `L(f)`. Within an open
interval `(l_i, l_{i+1})` every such comparison is constant, and within a point
band `{l_i}` every such comparison is constant. `unavailable` forces all
comparisons false. Conditions are built only from `all`/`any`/`not` over
comparisons, so they are constant too.

This claim is a required unit test, checked by exhaustive band enumeration over
each model in the fixture set.

### 4.3 Material change

A new immutable Situation version is written when any of these change:

1. the selected state;
2. any fact's band;
3. any fact's quality class;
4. any trigger match in the current evaluation, including an admission or
   suppression; a trigger opportunity is material even when the prior evaluation
   had no comparable trigger record;
5. a state candidacy starts, completes, or clears.

Fact movement *within* a band updates the `entity_state` projection and creates no
Situation version. Everything else does. Because of §4.2, that rule loses no
decision-relevant information.

## 5. Exact wake computation

Three things change without an event: an `age` comparison, a pending candidacy's
elapsed time, and a sustained trigger's eligibility. Every boundary is computable
exactly, so the runtime schedules it instead of polling.

```text
for each compiled age comparison over input x with arrival t(x) of the
latest-by-event-time admitted event:
    point-band boundary at literal l: wake = t(x) + l
    for gt or lte, post-point boundary: wake = t(x) + l + 1ns

for each pending candidacy c for state s with sustained_for D:
    wake(c)      = candidate_since(c) + D

for each sustained trigger r whose configured state is current:
    wake(r)      = max(state_entered_at + min_interval,
                       last_trigger_episode + min_interval,
                       episode_type_cooldown_release)
    if r was suppressed by a rolling-hour budget:
        wake(r)  = max(wake(r), exact_budget_release)

next_wake = earliest boundary strictly after the current evaluation instant
```

Rolling-hour membership is
`evaluation_instant - 1h < admitted_at <= evaluation_instant`; the oldest counted
episode therefore releases capacity exactly at `admitted_at + 1h`.

Candidacy wakes are not optional. Without them, a state entered on a silence
condition would become a candidate at the age crossing and then never be
re-evaluated, because no further age literal remains to cross — the transition
would hang pending forever.

Sustained-trigger wakes are equally non-optional. Without them, a six-hour review
fires only when some unrelated event happens after six hours, and may never fire
during the silence it is intended to review.

`next_wake` is stored in `entity_state.next_wake_ns` and indexed. The timer loop
selects `WHERE next_wake_ns < now` and evaluates each due entity with
`evaluation_instant = next_wake_ns` — the exact crossing, not the moment the
scanner happened to run. Recorded input at exactly the boundary is processed first
and may cancel the wake; replay uses the same ordering.

Consequences:

- no polling scan and no 10-second granularity;
- the evaluation instant for every timer evaluation is a pure function of the
  input, so live and replayed histories are identical;
- a band crossing can never be missed or double-counted;
- an entity with no age boundary, pending candidacy, or sustained trigger due is
  never woken.

In replay, the same computation runs against the virtual clock, and `replay --until`
advances the virtual clock past the last event so that silence is exercised
deterministically.

## 6. Compilation

`models validate` and `models install` compile the JSON document before it may be
activated. Compilation is pure: no clock, no network, no filesystem beyond reading
the document.

Ordered passes:

1. strict JSON Schema validation with duplicate-key rejection;
2. duration parsing and bounds checking;
3. input contract checking (unit/enum/min-max legality per value type);
4. window `max_samples` bounds;
5. reference resolution for inputs, windows, facts, states, triggers, episodes;
6. fact type and quality inference;
7. comparison operand type checking, including string literals against input enums;
8. age-operator and exact-nanosecond-literal checking;
9. unique-priority and exactly-one-default checking;
10. **`enter ⇒ stay` proof** per state (§6.1);
11. band-set construction per fact (§4.1) and band-count bounds;
12. `sustained_for`, cooldown, and trigger-wake bounds;
13. output-schema allowlist compilation and pointer resolution;
14. trigger, state, and episode reference checking;
15. **compatibility verdict** against the currently active version (§6.2);
16. normalized JSON serialization and SHA-256 digest generation.

Every rejection carries a JSON path and a stable machine-readable code. The
compiled representation contains indexes, typed expression nodes, and band tables.
It contains no user-supplied functions.

The model digest is `sha256:` plus the SHA-256 of RFC 8785 canonical JSON after
removing the informational top-level `$schema` field. The digest includes
`schema_version`, model `id`, and model `version`; changing identity therefore
changes the digest. Whitespace, object-key order, and equivalent JSON number
spellings do not. Array order is preserved because it identifies condition trace
paths and priority presentation.

### 6.1 Proving `enter ⇒ stay`

Hysteresis is only hysteresis if the stay condition is weaker than the entry
condition. V0 assumed this and never checked it (F-09). Over a finite band domain
it is decidable, in two stages.

**Stage 1 — disjunct matching.** Normalize `enter` and `stay` into disjunctions of
terms: the children of a top-level `any`, or the whole expression as a single
term. For each `enter` term `e`, search for one `stay` term `s` such that
`e ⇒ s`, enumerating only the bands of the facts that `e` and `s` mention. If
every `enter` term finds a match, the implication holds. The cross-product size is
computed before enumeration; a sub-problem above 100,000 combinations is skipped
rather than accidentally performing the work the cap exists to prevent.

Stage 1 is sound but incomplete: it will not prove a model where `enter` implies
the *disjunction* of stay terms without implying any single one. That is an
acceptable trade, because the incomplete case is also the case a human reviewer
cannot follow.

**Stage 2 — full enumeration.** Only if stage 1 fails: enumerate the cross product
of all bands of all facts in the state, capped at 100,000 combinations. Reject
with `hysteresis_uncheckable` if the product exceeds the cap.

Either stage rejects with the offending band assignment printed as a
counterexample:

```text
motor-warning@1.0.0 state "warning": enter does not imply stay
  vibration_max = 7.0 (point band)   vibration_count = 5 (point band)
  vibration_avg = 1.0   temperature_avg = 69.0   temperature_max = 94.0
  enter -> true      stay -> false
```

**Why two stages.** Naive whole-state enumeration is exponential in the number of
facts a state references, and it binds immediately on realistic models. The
[cold room](examples/coldroom-integrity.situation-model.json) `elevated` state
touches nine facts: whole-state enumeration needs **2,239,488** combinations, well
past the cap, so a one-stage compiler would refuse to install this project's own
conformance fixture. Stage 1 proves the same state in a largest sub-problem of
**324** combinations, because each term pair touches two or three facts. Measured
across both example models, stage 1 proves every state, and the largest
sub-problem anywhere is 2,688 combinations.

Refusing to install a model whose safety property cannot be verified is the
correct default for a system whose entire claim is determinism — but the verifier
has to be cheap enough that the default is not "everything is uncheckable."

### 6.2 Version compatibility and activation

Installing is not activating. On install, the compiler compares the candidate
against the currently active version for the same `entity_type` (F-08):

| Verdict | Condition |
|---|---|
| `compatible` | entity type and model-level lateness are unchanged; every retained event type has the same value contract; every active state ID still exists; and the default state ID is unchanged |
| `reset_required` | anything else |

`models activate <id>@<v> --mode=continue` requires `compatible`. Live entities
keep their state, but facts are recomputed and candidacies are cleared at each
entity's last recorded evaluation instant. The activation transaction writes one
`model_migration` Situation per live entity with both digests. Clearing candidacy
is intentional: time accumulated under one condition tree cannot satisfy another.
The current state's original entry time is retained for sustained-trigger cadence.

`models activate <id>@<v> --mode=reset --yes` is required otherwise. It writes one
`model_migration` Situation version per live entity, recording the previous state,
previous digest, new digest, and reason; each entity's state resets to the new
model's default, facts and candidacies are cleared, and open episodes are marked
`superseded_by_model_change`. Reset sets state-entry time to the migration
evaluation instant.

Every activation writes a `model_activations` row. A model version is immutable;
changing anything requires a new version.

### 6.3 Cold start

An entity with no `entity_state` row has previous state equal to the **model's
default state** (F-11). An entity whose very first event puts it in `warning`
therefore matches `from: ["normal", …]` and is diagnosed. This is a required test,
not an implementation detail.

## 7. Safety and resource limits

The compiler rejects:

| Limit | Bound |
|---|---|
| inputs | 32 |
| windows | 16 |
| facts | 128 |
| states | 32 |
| triggers | 32 |
| episode types | 16 |
| expression depth | 32 |
| expression nodes | 1,000 |
| distinct literals per fact | 32 |
| band combinations per hysteresis sub-problem | 100,000 |
| window duration | 24 h |
| window `max_samples` | 100,000 |
| model lateness allowance | 24 h |
| age comparison literal | 0 s through 10 Julian years |
| `sustained_for` | 24 h |
| `cooldown`, `min_interval` | 30 d |
| string enum entries | 64 |
| output-schema nodes / depth | 200 / 8 |

It also rejects unknown operators or fields, cyclic references, incompatible
operand types, string literals outside an input's enum, missing or multiple
default states, non-unique priorities, and any model that fails the hysteresis
proof.

All numeric literals must be finite. Age literals additionally obey the bound
above so converting seconds to signed nanoseconds and adding the input timestamp
cannot overflow; the final timestamp addition is checked as well.

Input event types must be unique within a model, numeric `min` must not exceed
`max`, state and trigger IDs must be unique, and a transition trigger's `from`
set must not contain its own `to` state. These are compiler checks because JSON
Schema cannot express the cross-reference invariants clearly.

Evaluation performs no user-controlled or unbounded loops, network access,
filesystem access, randomness, model calls, or wall-clock reads. Iteration over
expression nodes, facts, and bounded window samples is capped by the limits above.
Windows, event horizon, and evaluation instant are supplied to the evaluator as
explicit arguments.

## 8. Evaluation record

Every material Situation stores:

- Situation Model ID, semantic version, and digest;
- engine and compiler version;
- previous and selected state;
- fact values, bands, and quality;
- evidence provenance per fact: contributing count, query bounds, and at most 20
  representative event IDs. Operator-determining IDs (`max`, `min`, `latest`) come
  first, then the newest remaining contributors by the canonical event tuple; the
  immutable event log remains the complete source;
- state candidacies with elapsed and required durations;
- a tree of condition results;
- matched state ID and every trigger admission outcome with its reason.

```json
{
  "state": "warning",
  "matched": true,
  "sustained": {"required": "90s", "held": "92s", "satisfied": true},
  "condition": {
    "operator": "gte",
    "result": true,
    "left": {
      "fact": "vibration_max",
      "value": 7.2,
      "band": "i2",
      "quality": "ok",
      "evidence_event_ids": ["evt-000123", "evt-000131"]
    },
    "right": {"number": 7.0}
  }
}
```

Human prose is a projection generated from this structure by `explain`. The
structured record is the authoritative audit artifact.

## 9. Deployment lifecycle

```text
author JSON
  -> validate and compile          (models validate)
  -> install immutable version     (models install)
  -> inspect static difference     (models diff a b)
  -> replay both versions          (models compare --input trace.jsonl)
  -> read divergence report
  -> activate with an explicit mode (models activate --mode=continue|reset)
  -> retain prior version for audit and rollback
```

```text
agentic-stream models validate motor-warning.situation-model.json
agentic-stream models install  motor-warning.situation-model.json --db runtime.db
agentic-stream models diff     motor-warning@1.0.0 motor-warning@1.1.0 --db runtime.db
agentic-stream models compare  --input trace.jsonl --a motor-warning@1.0.0 --b motor-warning@1.1.0 --db runtime.db
agentic-stream models activate motor-warning@1.1.0 --mode=continue --db runtime.db
agentic-stream models show     motor-warning@1.1.0 --db runtime.db
```

`models compare` replays one trace through two versions in isolated temporary
databases and reports (F-21):

- the first Situation version at which the two histories diverge, with both records;
- per-state occupancy time for each version;
- trigger admissions present in one and not the other;
- suppression-reason counts;
- episode counts and cognition ratios.

Activation affects future processing only. V0.1 never rewrites existing Situations
except through an explicit `--mode=reset` migration.

## 10. Scope boundary

Configurable does not mean unlimited. V0.1 deliberately excludes:

- arbitrary arithmetic and user-defined functions;
- joins across entities;
- sequence and pattern matching;
- sliding, count, session, and calendar windows;
- percentile, variance, slope, and correlation operators;
- unit conversion;
- scripts, plugins, imports, and remote rule loading;
- dynamic rule modification by a model;
- conditions that execute actions.

New operators require a code change, semantic versioning, unit tests, replay
tests, a band-correctness proof, and a demonstrated product need. Domain behavior
stays configuration; engine capability stays reviewed code.
