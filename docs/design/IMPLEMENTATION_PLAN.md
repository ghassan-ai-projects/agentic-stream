# Implementation Plan

## 1. Delivery strategy

Deliver correctness in vertical slices. Each milestone must run end to end,
leave a deterministic fixture, and pass a release gate. Connectors, real models,
and user interfaces are added only after the core behavior exists with fakes.

Planning assumptions:

- one senior engineer full time;
- design and research are complete enough to begin Milestone 0;
- no existing production compatibility commitment;
- local/edge single-node is the first release boundary;
- estimates include implementation and automated tests, but not organization
  procurement or domain-expert availability.

Estimated path:

| Milestone | Outcome | Estimate |
|---|---|---:|
| M0 | Architecture runway and deterministic harness | 2 weeks |
| M1 | Event-time stream and Situation core | 4 weeks |
| M2 | Cognitive scheduling and bounded native episodes | 3 weeks |
| M3 | Governed action plane and crash recovery | 3 weeks |
| M4 | MQTT, operations, and pilot hardening | 3 weeks |
| M5 | Broker-backed beta and compatibility kit | 4–6 weeks |

M0–M4 produces the first production-capable local/edge release in roughly
15 engineer-weeks. M5 is a separate beta increment.

## 2. Release gates

### Gate A — deterministic foundation

- Replaying the same golden trace three times creates byte-identical canonical
  hashes for features, timers, Situation versions, and trigger evaluations.
- Tests use a virtual clock and fake IDs; no wall clock leaks into expected output.
- SituationSpec compilation is deterministic and rejects invalid references.

### Gate B — bounded cognition

- No event directly invokes a model.
- Debounce, cooldown, coalescing, expiration, and supersession have durable,
  explainable outcomes.
- An episode cannot exceed any configured budget.
- A canceled or superseded episode cannot produce an accepted Decision.

### Gate C — safe effects

- A model has no effector handle or credential.
- An Intent based on a stale Situation cannot become a Command.
- A crash at every outbox dispatch boundary produces no duplicate accepted effect.
- Unknown external outcome enters reconciliation, not automatic re-execution.
- Replay cannot load production effectors.

### Gate D — operational release

- Upgrade, backup, restore, disk-full, and unclean-shutdown runbooks pass.
- A 24-hour soak shows bounded memory, queue, timer, and database growth.
- Security review covers API binding, worker socket, secrets, event poisoning,
  capability tokens, and artifact retention.
- The predictive-maintenance acceptance suite passes with one command.

## 3. Milestone 0 — architecture runway

Objective: establish contracts, deterministic test infrastructure, repository
quality gates, and a non-networked vertical skeleton.

### Epic M0.1 — repository and build

Deliver:

- Go module and `cmd/agentic-stream`;
- package layout from the technical design;
- `Makefile` targets: `build`, `test`, `test-race`, `lint`, `generate`, `ci`;
- pinned tool versions;
- Go formatting, vet, static analysis, race, and vulnerability checks;
- cross-platform release build for Linux and macOS, amd64 and arm64;
- embedded database migrations and schema version command;
- Apache-2.0 license proposal confirmed before public release.

Tests:

- clean checkout builds without Python;
- generated files are reproducible;
- binary prints version, commit, build time, DB schema, and protocol versions.

### Epic M0.2 — foundational contracts

Implement:

- IDs with injectable generator;
- UTC timestamp and duration codecs;
- Event Envelope;
- Feature Event;
- Situation/Snapshot;
- Trigger Evaluation;
- Episode Request/Event/Outcome;
- Decision, Intent, Approval, Command, and Outcome;
- stable error envelope;
- canonical JSON and SHA-256 digest utilities.

Tests:

- round-trip JSON fixtures;
- unknown major version rejection;
- duplicate-key YAML rejection;
- stable canonical ordering;
- fuzz decoders with maximum-size enforcement.

### Epic M0.3 — SituationSpec compiler

Implement:

- YAML/JSON parsing;
- JSON Schema validation;
- typed reference resolution;
- duration and unit validation;
- CEL compile environment;
- normalized internal IR;
- canonical digest;
- human-readable diagnostic locations;
- `validate` and `config effective` CLI commands.

Tests:

- valid predictive-maintenance spec;
- missing source/window/feature references;
- invalid phase transition;
- cyclic operator topology;
- non-deterministic CEL function attempt;
- semantically equivalent YAML produces the same digest.

### Epic M0.4 — deterministic simulator and golden harness

Implement:

- virtual clock;
- deterministic ID generator;
- JSONL trace format;
- motor simulator with scenario seed;
- golden comparison writer/reader;
- fake EpisodeExecutor;
- fake Effector;
- crash/fault injection points.

Golden traces:

1. ordered normal operation;
2. duplicate events;
3. bounded out-of-order events;
4. late correction;
5. missing heartbeat;
6. noisy threshold oscillation;
7. burst overload;
8. hot key;
9. supersession;
10. crash around command dispatch.

Exit: Gate A harness exists, even if only ingress-normalization output is
produced at this stage.

## 4. Milestone 1 — event-time stream and Situation core

Objective: the same input always produces the same durable Situation history.

### Epic M1.1 — SQLite storage and local log

Implement migrations and repositories for:

- spec deployments;
- event log and inbox;
- connector/partition checkpoints;
- operator state;
- timers;
- lineage sets;
- current and immutable Situation versions;
- trigger evaluations;
- artifact metadata.

Operational behavior:

- WAL configuration and verification;
- transaction helpers with context cancellation;
- online backup API;
- integrity check in `doctor`;
- disk-space watermark;
- bounded read pool and single write coordinator.

Tests:

- migration up from every released schema version;
- transaction rollback on injected error;
- duplicate event unique constraint;
- process kill after each transaction stage;
- backup and restore equality;
- corrupt state detection.

### Epic M1.2 — ingress pipeline

Implement connectors in this order:

1. JSONL replay connector;
2. simulator connector;
3. HTTP batch ingest.

Ingress stages:

- authenticate source;
- enforce request/event size;
- decode schema;
- validate event/source/entity identity;
- normalize units;
- assign ingestion time and partition;
- preserve original payload reference if configured;
- quarantine rejected input;
- append accepted envelope.

Tests:

- malformed JSON and schema;
- timestamp skew;
- unsupported unit;
- duplicate IDs;
- mixed valid/invalid batch policy;
- backpressure when the local log is unavailable.

### Epic M1.3 — partition runtime and checkpoints

Implement:

- stable virtual partition hash;
- bounded input queues;
- worker ownership;
- serial transaction loop;
- partition stop/restart;
- checkpoint recovery;
- readiness failure on stuck partition.

Tests:

- all events for one entity remain ordered;
- two partitions make progress independently;
- no queue exceeds configured capacity;
- graceful shutdown drains only within deadline;
- unclean shutdown resumes at the last committed position.

### Epic M1.4 — watermarks and durable timers

Implement:

- per-source max event time;
- bounded-out-of-orderness watermark;
- partition minimum watermark;
- idle-source exclusion/rejoin;
- event-time timer heap;
- processing-time timer paging;
- virtual-time replay;
- timer restore.

Tests:

- monotonic watermark;
- out-of-order event before watermark;
- event after allowed lateness;
- idle source does not block forever;
- rejoining source creates late evidence without watermark regression;
- heartbeat fires without a new event;
- restart before/after timer due time.

### Epic M1.5 — operators

Implement:

- filter/map;
- tumbling and sliding windows;
- count window;
- decayed state;
- duration condition;
- aggregates: count/sum/min/max/mean/variance/stddev/RMS;
- first/last/delta/rate/slope;
- bounded quantile;
- correlation with explicit alignment;
- feature provenance.

Tests:

- table tests for each aggregate;
- property tests over chunk/batch boundaries;
- empty and one-sample windows;
- floating-point tolerances and canonical encoding;
- maximum retained state;
- late correction/retraction.

### Epic M1.6 — Situation reducer

Implement:

- occurrence identity;
- phase state machine;
- field reducers;
- hysteresis and minimum duration;
- evidence and contradiction sets;
- immutable version writes;
- current projection;
- provenance traversal;
- `situation list/show/explain`.

Exit tests:

- predictive-maintenance trace produces expected candidate/watch/warning states;
- missing heartbeat opens `sensor_health`, not `bearing_degradation`;
- a late correction creates a new corrected version;
- every field is explainable;
- Gate A passes across the complete deterministic stream plane.

## 5. Milestone 2 — cognitive scheduling and bounded episodes

Objective: cognition is rare, explainable, cancelable, and unable to mutate
stream or action state directly.

### Epic M2.1 — trigger engine

Implement:

- component scoring and reasons;
- meaningful-delta comparison;
- debounce timer;
- cooldown;
- completeness/confirmation rules;
- material supersession predicate;
- trigger evaluation persistence;
- `explain trigger`.

Tests:

- no cognition for normal repeated readings;
- one trigger after persistent multi-signal evidence;
- noisy oscillation does not trigger repeatedly;
- provisional trigger schedules on-time confirmation;
- low material delta is ignored with reason.

### Epic M2.2 — durable scheduler

Implement:

- scheduler item schema;
- admission dedupe key;
- bounded lane queues;
- global permits;
- one active episode per Situation;
- coalescing;
- expiration;
- cancellation/supersession;
- recovery of pending/running records;
- overload metrics.

Tests:

- deterministic priority ordering;
- durable queue after restart;
- coalesce five versions into the latest valid version;
- cancellation reaches executor;
- late result from canceled episode is rejected;
- capacity exhaustion does not slow deterministic processing.

### Epic M2.3 — snapshot and context assembler

Implement:

- deterministic snapshot;
- delta since last reasoned version;
- multi-resolution feature selection;
- bounded evidence/contradiction ranking;
- prior Decision/Outcome projection;
- tool capability projection;
- context budget report;
- snapshot hash.

Tests:

- fixed input produces fixed snapshot;
- raw windows are referenced, not dumped;
- size budget failure is diagnostic;
- restricted classification is excluded;
- evidence references resolve.

### Epic M2.4 — native episode executor

Implement:

- direct model-provider port;
- deterministic fake provider;
- one OpenAI-compatible streaming adapter;
- typed model event normalization;
- read-tool loop;
- argument validation;
- cancellation;
- all hard budgets;
- typed terminal outcomes;
- model/tool ledger;
- one structured-output repair attempt.

Do not implement:

- shell or arbitrary HTTP tools;
- long-term conversational memory;
- subagents;
- automatic provider marketplace/model discovery;
- generic plugin loading.

Tests:

- final Decision without tools;
- multiple bounded read tools;
- invalid arguments;
- missing tool;
- oversized result spills to artifact;
- provider timeout/retry;
- cancellation during model stream and tool call;
- model-call, token, tool, byte, cost, and wall-time exhaustion;
- repeated identical tool-call loop guard.

### Epic M2.5 — worker protocol

Implement:

- generated Go Protobuf types for the current v1 contract;
- protocol handshake and feature negotiation;
- local UDS gRPC server;
- capability token issuance;
- evidence-tool service;
- child-process supervision;
- Python reference fake worker;
- no legacy or N/N-1 compatibility fixture; unsupported versions fail closed.

Python package baseline:

- Python 3.12;
- Pydantic v2;
- `grpcio`;
- no LangGraph/LangChain dependency in the base install;
- optional extras for future adapters.

Exit:

- fake and native executors pass the same conformance suite;
- predictive-maintenance warning creates one bounded Decision;
- a material new version cancels the old episode;
- Gate B passes.

## 6. Milestone 3 — governed action plane and recovery

Objective: Decisions can lead to useful effects without allowing model output to
bypass deterministic authority.

### Epic M3.1 — Decision validator

Implement:

- schema validation;
- identity/snapshot/version checks;
- evidence visibility and existence;
- facts/inferences separation;
- confidence and expiry rules;
- allowed Intent types;
- normalized Decision hash;
- accepted/rejected persistence.

Tests:

- forged evidence;
- stale snapshot;
- unsupported Intent;
- oversized free text;
- duplicate Decision delivery;
- repair attempt still fails closed.

### Epic M3.2 — policy gateway

Implement the ordered policy chain:

1. Decision accepted;
2. Intent schema/capability;
3. current Situation freshness;
4. preconditions;
5. risk;
6. quota/rate;
7. approval;
8. Command/outbox.

Initial policies:

- R0 automatic;
- R1 automatic with rate limit;
- R2 approval;
- R3/R4 deny;
- replay always simulate/deny.

Tests:

- deny precedence;
- stale after approval;
- capability confusion;
- expired Intent;
- rate limit;
- policy version recorded.

### Epic M3.3 — approval workflow

Implement:

- approval request;
- CLI approve/deny;
- loopback HTTP approve/deny;
- identity and reason;
- expiry;
- freshness revalidation;
- audit and SSE events.

Tests:

- approve;
- deny;
- timeout;
- duplicate reply;
- non-owner reply;
- Situation changes during wait.

### Epic M3.4 — outbox and effectors

Implement:

- Intent and Command stores;
- atomic outbox creation;
- dispatcher with leases;
- idempotency;
- result/outcome events;
- reconciliation state machine;
- `simulated` effector;
- `maintenance.ticket` SQLite demo effector;
- optional allowlisted notification webhook only after demo effector passes.

Tests:

- kill before send;
- kill after send before acknowledgement;
- duplicate outbox delivery;
- timeout with known idempotent result;
- timeout with unknown result;
- manual reconciliation.

### Epic M3.5 — replay modes

Implement:

- deterministic replay;
- recorded-cognition replay;
- shadow executor replay;
- counterfactual simulated effector;
- isolated replay database;
- result comparison and first-divergence report;
- CLI `replay` and `compare`.

Tests:

- production effectors cannot be resolved in replay;
- recorded cognition reproduces accepted Decision;
- shadow model differs without creating Intent/Command;
- comparison locates first Situation divergence.

Exit:

- crash-injection matrix produces no duplicate ticket;
- recorded and shadow replays pass;
- Gate C passes.

## 7. Milestone 4 — MQTT, operations, and pilot hardening

Objective: one operator can run the system continuously on a local or edge host.

### Epic M4.1 — MQTT 5 connector

Implement:

- TLS and client identity;
- QoS mapping and documented guarantees;
- topic-to-event mapping;
- source clock/watermark policy;
- retained-message and duplicate handling;
- bounded reconnect/backoff;
- connector health and lag;
- integration tests against a real broker container.

### Epic M4.2 — HTTP/SSE API hardening

Implement:

- authentication token;
- loopback default;
- request IDs and idempotency keys;
- pagination and bounded query range;
- durable SSE resume for lifecycle events;
- rate and body limits;
- API version/openapi document;
- SDK smoke client.

### Epic M4.3 — telemetry and operations

Implement:

- OpenTelemetry spans and links;
- bounded-cardinality metrics;
- JSON logs with redaction;
- health/readiness;
- `doctor`;
- effective configuration output;
- graceful shutdown;
- online backup and restore commands;
- database/artifact retention jobs;
- disk watermarks;
- sample systemd and container packaging.

### Epic M4.4 — security hardening

Review and test:

- event poisoning and prompt injection corpus;
- capability-token scope/expiry;
- worker UDS permissions;
- remote worker disabled by default;
- secrets absent from logs, traces, Decisions, and artifacts;
- path traversal and symlink handling in artifact store;
- API non-loopback binding warnings;
- dependency audit and SBOM;
- fuzz parsers and protocol decoders.

### Epic M4.5 — performance and soak

Benchmarks:

- 1k, 10k, and burst events/s reference topology;
- 100k active keys;
- mostly-idle key/timer cardinality;
- late-correction storm;
- cognitive provider outage;
- approval backlog;
- WAL growth, checkpoint latency, and backup impact.

Tune only from profiles. Do not replace SQLite based on speculation.

Exit:

- documented reference-machine results;
- 24-hour soak;
- backup/restore and upgrade rehearsal;
- predictive-maintenance one-command demo;
- Gate D passes.

## 8. Milestone 5 — broker-backed beta

Objective: prove the contracts outside the embedded log without changing core
Situation or action semantics.

### Epic M5.1 — Kafka adapter

- `franz-go` connector;
- stable partition mapping validation;
- consumer-group offset recovery;
- at-least-once + inbox dedupe;
- transactional producer only where its real boundary helps;
- lag and rebalance metrics;
- broker failure/partition/rebalance tests.

### Epic M5.2 — tenant scheduling and quotas

- tenant quotas;
- weighted fair admission;
- per-tenant model/cost budget;
- per-tenant artifact and retention boundary;
- explicit statement that OS-process isolation is not hostile tenancy.

### Epic M5.3 — executor adapters

Prioritize from demonstrated demand:

1. LangGraph Python adapter;
2. LangChain agent adapter;
3. OpenClaw adapter;
4. Hermes adapter.

Every adapter must pass the same executor conformance suite. Do not import its
memory, session, or action semantics into the core.

### Epic M5.4 — compatibility kit

- connector TCK;
- EpisodeExecutor TCK;
- Effector TCK;
- historical event decoder fixtures;
- current-v1 protocol conformance tests; unsupported versions fail closed;
- schema compatibility report in CI;
- public golden stream corpus.

## 9. First 12 pull requests

This order is intentionally concrete:

1. `build: initialize Go module, CI, quality targets, version metadata`
2. `contracts: add IDs, clocks, canonical JSON, and v1 domain types`
3. `spec: compile SituationSpec v1 and predictive-maintenance fixture`
4. `testkit: add virtual clock, seeded simulator, JSONL, and golden runner`
5. `storage: add SQLite migrations, WAL profile, event log, and inbox`
6. `engine: add virtual partitions, bounded queues, checkpoints, and recovery`
7. `time: add watermarks, idle policy, durable timers, and late-data handling`
8. `operators: add bounded windows, aggregates, lineage, and property tests`
9. `situations: add reducer, lifecycle, immutable versions, and explain`
10. `cognition: add trigger evaluation, scheduler, debounce, and supersession`
11. `episodes: add snapshot assembler, fake executor, budgets, and ledger`
12. `demo: complete deterministic predictive-maintenance vertical slice`

PR 12 is the first product checkpoint. Real model and MQTT work start after its
review.

## 10. Test architecture

### 10.1 Test layers

| Layer | Purpose |
|---|---|
| Unit/table | codecs, reducers, policies, state transitions |
| Property/fuzz | parser safety, window invariants, canonicalization, dedupe |
| Golden stream | deterministic end-to-end temporal behavior |
| Contract/TCK | connectors, executors, effectors, worker protocol |
| Persistence | transactions, migrations, restart, corruption |
| Fault injection | process death and external uncertainty boundaries |
| Security | scope, injection, redaction, filesystem, API binding |
| Integration | SQLite, MQTT, model fake server, Kafka |
| Evaluation | trigger quality, Decision quality, cost, outcome utility |
| Performance/soak | throughput, cardinality, queues, leaks, WAL behavior |

### 10.2 Required golden scenarios

Each fixture includes input, virtual schedule, active spec digest, expected
feature stream, Situation history, trigger decisions, episode events, policy
outcomes, and canonical hash.

- ordered normal;
- duplicate delivery;
- out-of-order within allowance;
- late event after provisional Decision;
- missing heartbeat;
- source idle/rejoin;
- noisy phase boundary;
- burst overload;
- hot key;
- provider outage;
- tool timeout;
- invalid structured Decision;
- material supersession;
- crash before and after outbox send;
- replay effects disabled;
- schema upgrade and replay rebuild.

### 10.3 Race and leak tests

CI runs:

- `go test -race ./...`;
- goroutine-leak checks around cancellation and shutdown;
- repeated test runs for scheduler ordering;
- fuzz smoke corpus on every PR and longer scheduled fuzzing;
- deterministic hash comparison on Linux amd64 and arm64.

## 11. Definition of done

Every work item:

- has tests proportional to failure impact;
- preserves or adds a golden scenario when semantics change;
- documents schema, migration, and compatibility impact;
- has bounded inputs, outputs, queues, retries, and retention;
- emits useful diagnostics without secrets or high-cardinality metric labels;
- handles cancellation and shutdown;
- defines crash behavior before external side effects;
- updates `doctor` or runbooks when operational behavior changes;
- includes a self-review against the technical-design checklist;
- avoids adding an abstraction unless there are two real implementations or a
  trust/process boundary.

## 12. Review ownership

Minimum review domains:

| Change | Required review |
|---|---|
| Event-time, window, timer, reducer | temporal correctness |
| Database transaction, inbox/outbox | recovery/idempotency |
| Episode/tool/model | budget/cancellation/security |
| Intent/policy/effector | safety and stale-state behavior |
| Schema/protocol | compatibility |
| Connector | delivery guarantees and backpressure |
| Metrics/logs | cardinality and secret handling |

High-risk changes require a fault-injection test, not only code review.

## 13. Key risks and mitigations

| Risk | Early signal | Mitigation |
|---|---|---|
| Building a miniature Flink | growing operator/general topology scope | freeze v1 operator list; external-engine adapter later |
| Calling cognition too often | low event-absorption ratio, queue age | trigger precision metrics, debounce, cooldown, replay tuning |
| SQLite write ceiling | WAL stalls and p95 commit growth | profile/batch first; broker/distributed state only after evidence |
| Timer cardinality | heap/memory growth on idle keys | durable horizon paging and cardinality benchmark |
| Non-deterministic replay | hash drift | virtual clock/IDs, ordered reductions, canonical encoding |
| Provider-specific loop sprawl | branches inside core loop | normalize events in provider adapter; conformance tests |
| Policy bypass | generic tools or credentials reach worker | capability-scoped read tools and process boundary |
| Duplicate physical effects | unknown timeout retried | effector contract, idempotency, reconciliation |
| Spec language becomes code | complex CEL and hidden state | restricted functions, built-ins, compiler diagnostics |
| Premature plugin ecosystem | unstable contracts | keep packages internal until one release |
| Reference-project scope creep | copying channels/sessions/graphs | enforce product non-goals and adapter boundary |

## 14. Decisions required from the product owner

These do not block M0–M2 and should be resolved before the named milestone:

| Decision | Needed by | Default |
|---|---|---|
| Public repository/module namespace | M0 release packaging | keep module private until chosen |
| License confirmation | first public commit | Apache-2.0 |
| First real model/provider | M2 provider adapter | one OpenAI-compatible endpoint |
| First MQTT broker for certification | M4 | Mosquitto for tests |
| First real ticket/notification integration | M4 pilot | generic allowlisted webhook after demo effector |
| Pilot hardware and event rates | M4 benchmark | documented developer machine + simulator |
| Kafka vs NATS as first durable broker | M5 | Kafka if enterprise stream integration is primary; NATS if edge simplicity is primary |

## 15. Success measures

Do not use “number of agents” or “events sent to an LLM.”

Track:

- raw-event absorption ratio without cognition;
- precision/recall of useful Situation transitions;
- trigger precision and missed-important-situation rate;
- event horizon to durable Situation latency;
- trigger to validated Decision latency;
- cognition cost per resolved Situation;
- superseded episode rate;
- provisional-evidence Decision rate;
- duplicate and unknown external effects;
- replay reproducibility;
- Decision acceptance and outcome utility;
- setup time from clean machine to running demo;
- connector/executor/effector conformance pass count.

The product is improving when it reasons less often, explains more clearly, and
produces fewer unsafe or duplicate effects.
