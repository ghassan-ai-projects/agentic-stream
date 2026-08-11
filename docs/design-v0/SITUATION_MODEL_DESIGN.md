# Agentic Stream V0 — Configurable Situation Model

## 1. Decision

Domain behavior is injected as configuration from the first release.

Go code implements a generic deterministic model runtime. Situation Models
configure:

- accepted event inputs and units;
- event-time windows and lateness;
- derived facts;
- Situation states and priority;
- entry and stay conditions for hysteresis;
- cognition triggers;
- episode objective, instruction, and structured output schema.

Thresholds, durations, state names, event names, condition structure, cognition
transitions, domain prompt text, and Decision shape are not compiled into the
binary.

V0 does not accept arbitrary code or a general expression language. The
injected model is a typed JSON document with a closed operator set.

## 2. Why a constrained AST

CEL, JavaScript, Python, and user plugins would provide flexibility, but they
would also introduce type ambiguity, non-determinism, security exposure, and a
larger debugging surface.

A JSON expression tree is less pleasant to write manually but has useful V0
properties:

- strict schema validation;
- no parser ambiguity;
- static fact and type checking;
- complete operator allowlisting;
- deterministic canonical hashing;
- evaluation traces for every node;
- straightforward replay;
- no hidden I/O or clock access.

An authoring format can be improved after the semantic model is proven.

## 3. Situation Model structure

The machine contract is
[situation-model-v0.schema.json](contracts/situation-model-v0.schema.json). The
complete injected motor example is
[motor-warning.situation-model.json](examples/motor-warning.situation-model.json).

Top-level sections:

```json
{
  "schema_version": "0.1",
  "id": "motor-warning",
  "version": "1.0.0",
  "entity_type": "motor",
  "inputs": {},
  "windows": {},
  "facts": {},
  "states": [],
  "episode_types": {},
  "cognition_triggers": []
}
```

### Inputs

Inputs map domain names to event types and value contracts:

```json
{
  "vibration": {
    "event_type": "motor.vibration",
    "value_type": "number",
    "unit": "mm/s"
  }
}
```

The engine rejects an event whose configured input has the wrong value type or
unit.

### Windows

V0 supports fixed trailing windows only:

```json
{
  "recent": {
    "duration": "5m",
    "lateness_allowance": "2m"
  }
}
```

Durations use a restricted Go-duration subset: integer `ms`, `s`, `m`, or `h`.
Durations must be positive and within configured engine limits.

### Facts

V0 supports four fact operators:

```text
avg(input, window)       -> number or unavailable
max(input, window)       -> number or unavailable
latest(input)            -> number/string or unavailable
absent(input, timeout)   -> boolean
```

Facts declare their operator and references. References must resolve during
compilation. Numeric aggregates may only use numeric inputs.

### Conditions

Conditions are typed expression nodes. The closed V0 set is:

```text
all, any, not
gte, gt, lte, lt, eq, neq
fact, number, string, boolean
```

Examples:

```json
{"gte": [{"fact": "vibration_max"}, {"number": 7.0}]}
```

```json
{
  "all": [
    {"gte": [{"fact": "vibration_avg"}, {"number": 5.0}]},
    {"gte": [{"fact": "temperature_avg"}, {"number": 80.0}]}
  ]
}
```

Comparisons with an unavailable fact evaluate to `false` and record
`fact_unavailable` in the trace. No implicit numeric or string coercion occurs.

### States and hysteresis

Each state has a unique priority, an `enter` condition, and optionally a `stay`
condition:

```json
{
  "id": "warning",
  "priority": 100,
  "enter": {"gte": [{"fact": "vibration_max"}, {"number": 7.0}]},
  "stay": {"gte": [{"fact": "vibration_max"}, {"number": 6.0}]}
}
```

One lowest-priority state must have `default: true`. It has no conditions.

Evaluation algorithm:

1. Evaluate `enter` for states with higher priority than the current state.
2. Select the first matching state in descending priority.
3. If none matches, evaluate the current state's `stay`.
4. If it matches, keep the current state.
5. Otherwise evaluate all state `enter` conditions in descending priority.
6. If none matches, select the default state.

This makes escalation possible while preserving configurable hysteresis.

### Cognition triggers

V0 supports transition triggers:

```json
{
  "id": "diagnose-warning",
  "from": ["normal", "watch"],
  "to": "warning",
  "episode_type": "diagnosis"
}
```

The evaluator emits a durable trigger result. It never calls the model itself.

### Episode types

An episode type injects the domain-specific cognition contract:

```json
{
  "diagnosis": {
    "objective": "Diagnose the motor condition.",
    "instruction": "Use only supplied evidence and return JSON.",
    "output_schema": {
      "type": "object",
      "required": ["diagnosis", "evidence_event_ids"]
    },
    "evidence_reference_pointer": "/evidence_event_ids"
  }
}
```

The runtime adds only generic safety instructions, the frozen Situation
snapshot, and the selected output schema. It validates output against the
injected schema and verifies that every ID at the configured JSON Pointer exists
in the snapshot.

## 4. Compilation

`models validate` and `models install` compile the JSON document before it can
be activated.

Compilation performs:

1. strict JSON Schema validation;
2. duplicate-key rejection;
3. duration parsing and bounds checking;
4. input, window, fact, and state reference resolution;
5. fact-type inference;
6. expression operand type checking;
7. unique state-priority checking;
8. exactly-one-default-state checking;
9. episode output-schema compilation;
10. trigger, state, and episode reference checking;
11. normalized JSON serialization and SHA-256 digest generation.

The compiled representation contains indexes and typed expression nodes. It
contains no functions supplied by the user.

Activation stores the original document, normalized document, digest, and
compiler version in SQLite. A model version is immutable. Changing anything
requires a new version.

## 5. Evaluation result

Every material Situation stores:

- Situation Model ID, semantic version, and digest;
- engine/compiler version;
- previous and selected state;
- derived fact values and availability;
- evidence event IDs per fact;
- a tree of condition results;
- matched state and cognition-trigger IDs.

Example trace fragment:

```json
{
  "state": "warning",
  "matched": true,
  "condition": {
    "operator": "gte",
    "result": true,
    "left": {
      "fact": "vibration_max",
      "value": 7.2,
      "evidence_event_ids": ["evt-123"]
    },
    "right": {"number": 7.0}
  }
}
```

The explanation is generated from structured evaluation data. Human prose is a
projection, not the authoritative audit record.

## 6. Deployment lifecycle

```text
author JSON
  -> validate and compile
  -> install immutable version
  -> replay against a trace
  -> inspect Situation diff
  -> activate for new events
  -> retain prior version for audit and rollback
```

V0 CLI:

```text
agentic-stream models validate motor-warning.situation-model.json
agentic-stream models install motor-warning.situation-model.json --db runtime.db
agentic-stream models replay motor-warning@1.0.0 --input trace.jsonl
agentic-stream models activate motor-warning@1.0.0 --db runtime.db
agentic-stream models show motor-warning@1.0.0 --db runtime.db
```

Activation affects future processing. V0 does not automatically rewrite
existing Situations. Replay into a separate database is required to compare a
new model against history.

## 7. Safety and resource limits

The compiler rejects:

- unknown operators or fields;
- recursive or cyclic references;
- more than 32 expression levels;
- more than 1,000 expression nodes;
- more than 32 inputs, 16 windows, 128 facts, 32 states, or 32 triggers;
- windows over 24 hours;
- absence timeouts over 24 hours;
- incompatible operand types;
- missing or multiple default states.

Evaluation has no loops, network, filesystem, randomness, model calls, or wall
clock access. The engine supplies the event horizon and arrival clock as explicit
inputs.

## 8. Scope boundary

Configurable does not mean unlimited.

V0 deliberately excludes:

- arbitrary arithmetic and user functions;
- joins across entities;
- sequence and pattern matching;
- sliding-count or session windows;
- percentile and statistical operators;
- scripts, plugins, imports, and remote rule loading;
- dynamic rule modification by a model;
- model conditions that execute actions.

New operators require a code change, semantic versioning, unit tests, replay
tests, and an explicit product need. Domain behavior remains configuration;
engine capability remains reviewed code.
