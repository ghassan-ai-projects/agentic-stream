# Agentic Stream V0 — Implementation Plan

## 1. Delivery rule

Time-box V0 to 25 working days for one senior engineer. Build one vertical
slice at a time. Every week must end with a runnable artifact.

Do not build a reusable subsystem unless the current acceptance test requires
it.

## 2. Release gates

### Gate A — deterministic Situation

- Situation Models are strict external documents validated before activation.
- The motor model runs without motor-specific runtime code or database columns.
- A second synthetic model runs without recompiling or branching the runtime.
- The same trace replayed three times produces identical canonical Situation
  hashes.
- Duplicate events do not change aggregates or Situation versions.
- In-allowance out-of-order events correct the active window.
- Too-late events are stored but ignored by the model evaluator.
- Configured stay conditions prevent repeated state changes around a boundary.
- Heartbeat silence is deterministic under the virtual clock.

### Gate B — selective cognition

- Only a transition matching an injected cognition trigger creates an episode.
- Repeated warning evidence creates no additional episode.
- The model receives only the frozen, bounded snapshot.
- Invalid output creates no Decision.
- Every accepted evidence reference exists in the snapshot.
- A provider outage does not stop event processing.

### Gate C — recovery and operation

- Killing the process at each transaction boundary loses no accepted event.
- A stale running episode resumes after restart.
- There is at most one accepted Decision per episode.
- The server is loopback-only by default and has request size/time limits.
- The 24-hour soak meets the V0 capacity targets.

### Gate D — product proof

- The one-command demo shows `normal -> watch -> warning -> normal`.
- The warning has exactly one structured diagnosis.
- The Decision cites real evidence and adds useful interpretation beyond the
  matched deterministic model branch.
- An operator can understand the timeline from CLI output alone.

## 3. Week 1 — injected model foundation

Outcome: an external Situation Model compiles into a deterministic typed form.

### Day 1 — repository skeleton

Implement:

- Go module and `cmd/agentic-stream`;
- package layout from the technical design;
- build, test, race-test, format, and vet targets;
- version command;
- deterministic ID and fake clock test helpers.

Tests:

- clean build;
- race test target;
- version output fixture.

### Day 2 — generic contracts and SQLite

Implement:

- SituationModel, Event, EntityState, Situation, Episode, and Decision structs;
- strict JSON decoding and validation;
- embedded schema migration;
- SQLite open/configure/doctor functions;
- six concrete storage query groups.

Tests:

- JSON valid/invalid tables;
- migration into an empty database;
- foreign-key enforcement;
- duplicate event constraint;
- rollback on injected transaction error.

### Day 3 — Situation Model validator and compiler

Implement:

- JSON Schema validation and duplicate-key rejection;
- duration and reference resolution;
- typed fact and condition compilation;
- state-priority, default-state, trigger, and episode validation;
- normalized JSON and immutable digest;
- `models validate`, `install`, `activate`, and `show`.

Tests:

- unknown operator and field;
- invalid duration and reference;
- incompatible comparison operands;
- duplicate priority and missing default;
- invalid trigger or episode reference;
- semantically identical input produces the same digest.

### Day 4 — generic fact and state evaluator

Implement:

- `avg`, `max`, `latest`, and `absent`;
- typed condition tree evaluation;
- priority, entry, stay, and default-state algorithm;
- complete structured evaluation trace;
- material-change detection;
- immutable Situation versions.

Tests:

- every operator and unavailable-fact behavior;
- escalation, stay hysteresis, de-escalation, and default;
- nested `all`, `any`, and `not`;
- no version for aggregate movement in the same band;
- evidence IDs belong to the active window.

### Day 5 — event processor and replay

Implement:

- synchronous `ProcessEvent`;
- active-model lookup and input mapping;
- deduplication, event horizon, and configured lateness;
- JSONL trace reader;
- virtual clock;
- `replay`, `show`, and initial `demo` commands;
- golden Situation output.

Gate:

- pass Gate A except absence timers and the second synthetic model;
- demo `normal -> watch -> warning -> normal` without cognition.

## 4. Week 2 — live input and temporal completion

Outcome: the same injected model works under HTTP and absence timers.

### Day 6 — HTTP server

Implement:

- `POST /v0/events`;
- current motor and history endpoints;
- strict content type and body limit;
- server timeouts;
- loopback default;
- bearer-token guard for non-loopback binding.

Tests:

- handler success, duplicate, malformed body, oversized body, and DB failure;
- live input assigns arrival time;
- non-loopback configuration without token fails at startup.

### Day 7 — timer scanner

Implement:

- active-motor scan;
- heartbeat freshness calculation;
- ten-second live scanner;
- replay `--until`;
- timer-originated material Situation versions.

Tests:

- stale at just over two minutes;
- not stale at exactly two minutes;
- fresh heartbeat clears watch using hysteresis;
- restart derives silence from durable last arrival;
- virtual and wall-clock paths share evaluator behavior.

### Day 8 — recovery and operational health

Implement:

- graceful shutdown;
- `/healthz`, `/readyz`, and `/debug/vars`;
- SQLite integrity check;
- stale episode recovery query;
- structured logging and correlation fields.

Tests:

- shutdown during input;
- database unavailable and corrupt;
- readiness before/after migration;
- counters update once per accepted event.

### Day 9 — failure injection

Add test-only crash points:

1. after event insert;
2. after state update;
3. after Situation insert;
4. after episode insert;
5. before commit;
6. after commit.

Verify restart result for every point. No partial committed mutation is allowed.

### Day 10 — review and simplify

- run race tests and fuzz the event decoder;
- profile the active-window query;
- remove unused abstractions and configuration;
- document the event and Situation Model contracts;
- close Gate A and the event-processing parts of Gate C.

Do not begin model integration with a failing deterministic gate.

## 5. Week 3 — bounded configured cognition

Outcome: a configured transition produces one Decision validated against the
injected episode schema.

### Day 11 — episode snapshot and admission

Implement:

- configured transition-trigger admission;
- unique episode identity;
- canonical bounded snapshot;
- prior two Situations and at most 20 events;
- pending worker wake-up.

Tests:

- unmatched transitions do not admit;
- repeated state does not admit;
- a second configured matching transition admits a new episode;
- snapshot event and byte caps;
- episode insert is atomic with Situation transition.

### Day 12 — fake model and validator

Implement:

- `ReasoningClient`;
- deterministic fake;
- injected output-schema compilation and validation;
- configured evidence-reference-pointer validation;
- accepted Decision transaction.

Tests:

- valid response;
- malformed JSON;
- output-schema type, enum, length, and unknown-field violations;
- hallucinated evidence ID;
- oversized output;
- duplicate completion attempt.

### Day 13 — HTTP model adapter

Implement one configured JSON/HTTP adapter:

- fixed request and response mapping;
- authorization header;
- context cancellation and 30-second timeout;
- 16 KiB response cap;
- status and error normalization;
- secret-safe logs.

Tests use `httptest.Server` for:

- success;
- timeout;
- connection close;
- `429`;
- `500`;
- oversized body;
- invalid response.

Do not add a second provider.

### Day 14 — durable worker

Implement:

- single episode worker;
- atomic pending-to-running claim;
- completed and failed terminal states;
- startup recovery of stale running work;
- manual `retry-episode`.

Tests:

- process restart before call;
- process restart after claim;
- provider failure while new events continue;
- one Decision under concurrent wake-ups;
- retry is explicit and retains its accumulated attempt count.

### Day 15 — cognition demo

- connect fake and real-provider configuration paths;
- add Decision API and CLI rendering;
- show generic facts, evaluation trace, output JSON, and evidence references;
- complete Gate B.

## 6. Week 4 — correctness and hardening

Outcome: a measured release candidate for the injected motor model.

### Days 16–17 — correctness suite

Create committed traces for:

1. normal operation;
2. duplicate burst;
3. out-of-order correction;
4. too-late input;
5. noisy watch threshold;
6. immediate vibration warning;
7. combined heat and vibration warning;
8. high-load temperature adjustment;
9. missing heartbeat;
10. crash and recovery.

Each trace has golden Situations and expected episode count. Model tests use the
fake.

### Day 18 — security and resource bounds

- verify loopback and bearer-token behavior;
- run decoder fuzzing;
- inspect logs for secrets and full prompts;
- verify file permissions;
- enforce DB, request, snapshot, and response limits;
- benchmark 100 events/second and 1,000 active motors.

### Day 19 — soak and operator experience

- run the 24-hour accelerated or wall-clock soak;
- inspect memory, goroutine, file-descriptor, DB, and WAL growth;
- exercise database backup and restore;
- run the demo from a clean checkout;
- improve CLI output until the timeline is understandable without SQL.

### Day 20 — model lifecycle and replay comparison

- reject in-place modification of an installed model version;
- install a changed version alongside the active version;
- replay both versions into isolated databases;
- compare Situation and trigger histories;
- activate and roll back without changing model content;
- verify every event and Situation retains its exact model digest.

## 7. Week 5 — domain-independence proof and release

Outcome: demonstrate that domain behavior is injected, then make the product
decision.

### Days 21–22 — second synthetic Situation Model

Create an unrelated synthetic cold-storage model using different:

- event names and units;
- windows and lateness;
- fact names;
- state names;
- transition structure;
- cognition trigger;
- objective and Decision output schema.

Run it through the unchanged binary and database schema. Any code branch,
database column, or API path added specifically for cold storage fails this
acceptance test.

### Day 23 — final soak and security review

- run both models concurrently for 24 hours;
- inspect memory, goroutine, file-descriptor, database, and WAL growth;
- fuzz model and event decoders;
- confirm models cannot invoke I/O or bypass resource limits;
- exercise backup and restore.

### Day 24 — operator and product evaluation

- run both demos from a clean checkout;
- compare model versions using replay;
- have a domain reviewer score the motor Decisions;
- document gaps in model authoring, explanations, and diagnosis usefulness.

### Day 25 — release and decision review

Deliver:

- versioned binary;
- example JSONL trace;
- sample configuration;
- concise operator runbook;
- benchmark and soak results;
- product-evaluation report.

Close Gates C and D.

## 8. Pull request sequence

Keep changes reviewable in this order:

1. `build: bootstrap Go binary and deterministic testkit`
2. `storage: add generic six-table SQLite schema and migrations`
3. `models: validate and compile injected Situation Models`
4. `models: evaluate facts, conditions, states, and traces`
5. `runtime: validate, deduplicate, and persist configured events`
6. `replay: add virtual-clock JSONL runner and golden tests`
7. `api: add loopback ingest and generic entity inspection`
8. `runtime: add configured absence timers and recovery checks`
9. `cognition: add configured trigger admission and frozen snapshots`
10. `cognition: validate Decisions against injected output schemas`
11. `provider: add one bounded HTTP model adapter`
12. `worker: add durable cognition processing and retry`
13. `conformance: run a second domain model without runtime changes`
14. `release: complete demos, soak, security, and runbook`

No PR may introduce a generic plugin, operator, graph, policy, broker, or worker
framework.

## 9. Required test layers

| Layer | Purpose |
|---|---|
| Unit tables | compiler, fact operators, state semantics, validation, canonicalization |
| Property/fuzz | model/event decoders, deduplication, evaluator emits only configured states |
| SQLite integration | model lifecycle, transactions, constraints, recovery, window queries |
| HTTP integration | input limits, status behavior, auth, model failure |
| Golden replay | deterministic Situation and episode histories |
| Crash tests | atomicity and stale episode recovery |
| Benchmark/soak | query latency, memory, WAL, goroutine stability |

Run unit and integration tests under the Go race detector in CI.

## 10. Definition of done

A work item is done only when:

- behavior and failure behavior are implemented;
- automated tests cover boundary cases;
- errors include useful stable context without secrets;
- all queues, payloads, queries, and external calls are bounded;
- race, vet, format, and unit/integration checks pass;
- user-visible contracts are documented;
- no excluded V0 capability was introduced indirectly.

## 11. Stop/go decision

After the demo, compare two operator outputs for at least 20 labeled warning
scenarios:

1. deterministic Situation and matched model evaluation only;
2. Situation plus model diagnosis.

Have a domain reviewer score:

- correct diagnosis;
- useful alternative hypotheses;
- evidence grounding;
- recommended next-step usefulness;
- harmful overconfidence.

Proceed beyond V0 only if the diagnosis materially improves the operator
decision and the false-confidence rate is acceptable. If it does not, improve
the injected model or evidence, or stop. More orchestration will not fix a model
that adds no value.
