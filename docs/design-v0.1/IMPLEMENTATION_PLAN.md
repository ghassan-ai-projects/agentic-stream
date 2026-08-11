# Agentic Stream V0.1 — Implementation Plan

## 1. Delivery rule

**25 build days plus 5 evaluation days**, one senior engineer. Build one vertical
slice at a time. Every week ends with a runnable artifact.

Two rules that V0 lacked and that the schedule depends on:

1. **Week 6 is not compressible.** The evaluation is the deliverable that answers
   the product question; the build exists to enable it. If the build slips, cut
   from §9, never from Week 6.
2. **Do not build a reusable subsystem unless the current acceptance test requires
   it.** Unchanged from V0, and still the most useful sentence in the plan.

Two Situation Models exist from Day 3. Every subsequent day runs both in CI.

## 2. Release gates

### Gate A — deterministic Situation (end of Week 2)

- Situation Models are strict external documents validated before activation.
- The motor model runs with no motor-specific runtime code, column, or API path.
- The cold room model runs on the same binary and schema with no changes.
- The same trace replayed three times produces identical canonical hashes.
- **Ingest live → `trace export` → replay produces identical hashes** (F-06).
- Duplicate events change nothing; ID reuse returns `409` and is counted.
- In-allowance out-of-order events correct the active window.
- Late and out-of-contract events are persisted, non-admitted, and visible.
- Situation versions are created only by the material reasons in the contract
  (state, band, quality, candidacy, or trigger match), and the exhaustive
  band-property test passes for both models.
- The compiler rejects a model whose `stay` is not implied by `enter`, with a
  counterexample.
- `sustained_for` delays a transition and clears on a falling edge.
- Silence is exercised through exact wakes, not polling, under the virtual clock.
- Cold start: an entity whose first event is a warning is diagnosed.

### Gate B — selective cognition (end of Week 3)

- Only an admitted trigger creates an episode.
- Repeated warning evidence inside a cooldown creates no episode and records
  `suppressed_cooldown` durably.
- A `sustained` trigger re-diagnoses a long-lived warning at `min_interval` and
  not before.
- Budget exhaustion records `suppressed_budget` and does not stop ingestion.
- The model receives only the frozen, bounded snapshot, and it contains prior
  Decisions and sanitized non-admitted evidence.
- Output failing the injected schema creates no Decision.
- A hallucinated evidence ID fails the episode and is counted separately.
- A provider outage does not affect readiness or event processing.
- Cognition ratio is reported and is ≤ 0.5 % on the golden traces.

### Gate C — recovery and operation (end of Week 4)

- Killing the process at each of six transaction boundaries loses no persisted
  event and produces no partial mutation.
- A stale running episode resumes after restart; at most one Decision per episode.
- `models activate --mode=continue` is refused on an incompatible pair;
  compatible continue and explicit reset each write one `model_migration`
  Situation per live entity, with their distinct carry/reset semantics.
- `models compare` reports first divergence between two versions over one trace.
- The server is loopback-only by default, `expvar` is off by default, and request
  size and time limits hold.
- The 24-hour soak meets the capacity targets.

### Gate D — product proof (end of Week 6)

- The one-command demo shows `normal → watch → warning → normal` with one
  diagnosis, on both models.
- An operator reconstructs the full timeline, every state change, and every
  suppressed trigger from `explain` alone, with no SQL.
- The three-arm evaluation completes on ≥ 70 scenarios, with three repetitions
  for model arms and blinded scoring.
- The pre-registered decision rule returns a verdict, and the report contains
  every case where arm B beat arm C.

## 3. Week 1 — injected model foundation and the conformance fixture

Outcome: two unrelated external Situation Models compile into a deterministic
typed form on one binary.

### Day 1 — repository skeleton

- Go module and `cmd/agentic-stream`; package layout from the technical design.
- Build, test, race, format, and vet targets; version command.
- Deterministic ID helpers and a fake clock.
- **Write both example models before the runtime exists** (F-05). They are
  requirements documents at this point, not fixtures. The cold room model must be
  authored without reference to any Go code.

Tests: clean build, race target, version fixture, both JSON documents parse.

### Day 2 — contracts and SQLite

- `SituationModel`, `Event`, `EntityState`, `Situation`, `Episode`, `Decision`.
- Strict JSON decoding with unknown-field rejection.
- Embedded migrations; open/configure/doctor; seven concrete query groups.

Tests: valid/invalid decode tables, migration into an empty database,
foreign-key enforcement, duplicate-event constraint, rollback on injected error.

### Day 3 — validator and compiler, passes 1–9

- JSON Schema validation with duplicate-key rejection and **remote resolution
  disabled**; preload the three embedded schemas into a local registry by their
  exact `$id` values so trace-record references never touch the network.
- Duration parsing and bounds; input contract rules (unit/enum/min-max);
  model-level lateness and mandatory explicit budgets.
- Reference resolution; fact type and quality inference.
- Comparison operand typing, including string literals against input enums.
- Age comparisons restricted to ordered operators and exact nanosecond literals.
- Unique priority, exactly one default.
- `models validate`, `install`, `show`.

Tests: unknown operator and field; invalid duration; unresolvable reference;
incompatible operands; string literal outside enum; duplicate priority; missing
and multiple defaults; whitespace, object-key order, and equivalent JSON number
spellings produce the same RFC 8785 digest.

### Day 4 — bands, hysteresis proof, output-schema allowlist

- Band-set construction per fact, with point bands at literals (F-07).
- `enter ⇒ stay` proof, **two-stage**: disjunct matching first, whole-state
  enumeration only as a fallback, with counterexample output and the
  100,000-combination cap per sub-problem (F-09). One-stage enumeration is not
  viable — it needs 2.2 M combinations on the cold room `elevated` state and would
  reject our own conformance fixture.
- Output-schema keyword allowlist, node and depth caps, pointer resolution,
  `confidence_pointer` (F-16).
- Compatibility verdict against the active version (F-08).

Tests: the band property — for every pair of values in the same band, every
comparison in the model agrees — asserted exhaustively over both models; both
example models prove at stage 1 with every sub-problem under 3,000 combinations;
a model with an inverted `stay` is rejected with a counterexample naming the
offending point band; `$ref`, `allOf`, and an oversized schema are all rejected; a
compatible and an incompatible successor version each produce the right verdict.

A Python reference implementation of §4.1 and §6.1 exists and has been run against
both example models; use it as the oracle for the Go port.

### Day 5 — fact and state evaluator

- `avg`, `max`, `min`, `count`, `latest`, `age`, each with a quality class.
- Typed condition-tree evaluation with a complete trace.
- Candidacy tracking for every non-current state that declares
  evaluation-instant `sustained_for`.
- Priority → sustained → stay → lower priority → default selection.
- Durable state-entry time for sustained-trigger scheduling.
- Material-change detection from bands, quality, candidacy, and trigger outcomes.
- Immutable Situation versions and canonical hashing.
- Exact next-wake computation (F-06).

Tests: every operator including empty windows and never-seen inputs;
`unavailable` forces every comparison false; escalation, stay hysteresis,
de-escalation, default; nested `all`/`any`/`not`; no version for movement inside a
band; a version for a quality change at the same value; evidence IDs belong to the
active window; latest-event tuple ties are deterministic; an older out-of-order
heartbeat cannot reset `age`; next wake matches the computed crossing.

**Both models must pass this suite. Any test that names a motor is a bug.**

## 4. Week 2 — ingest, time, and the operator surface

Outcome: the same models work under replay and HTTP, and a human can read the
result.

### Day 6 — event processor and replay

- Synchronous `ProcessEvent`; active-model lookup and input mapping.
- Admission (`admitted`, `late_beyond_allowance`, `value_out_of_contract`) with
  full persistence (F-14).
- Deduplication with payload-hash conflict detection (F-13).
- Event horizon and lateness; `max_samples` truncation with `partial` (F-12).
- Discriminated JSONL reader for event and model-activation records with
  a non-mutating full validation pass, `arrival_time` validation (F-24), and a
  virtual clock.
- `replay`, `show`, initial `demo`; golden Situation output.

### Day 7 — HTTP server

- `POST /v0.1/events`, entity, history, decisions, episode, and stats endpoints.
- Strict content type, body limit, server timeouts, loopback default.
- Bearer-token guard for non-loopback; `expvar` behind `--debug-vars` (F-18).
- `/readyz` covering migrations and the write path only (F-22).

Tests: success, duplicate, reuse, malformed, oversized, DB failure; live input
assigns and persists arrival time and rejects a client-supplied one; non-loopback
without a token fails at startup.

### Day 8 — exact wake scheduler and trace export

- `next_wake_ns` maintenance for age-band boundaries, candidacy completion, and
  sustained-trigger eligibility; due-entity index.
- Wake evaluation with `evaluation_instant = next_wake_ns`.
- `replay --until`; wake-originated Situation versions.
- `trace export`, including the runtime admission config, immutable model
  activation records, and a final inclusive `trace_end` horizon (F-06).

Tests: crossing at exactly the literal and one nanosecond either side; recorded
input at a boundary precedes and may cancel that wake; restart recomputes the next
wake from durable arrival times; an entity with no `age` facts or trigger timers is
never woken; activation drains only earlier due wakes; virtual and wall-clock paths
produce identical evaluator output.

### Day 9 — `explain` and `watch`

- Structured trace to human projection (F-20), matching the format in
  [TECHNICAL_DESIGN.md §11.2](TECHNICAL_DESIGN.md).
- `explain` over CLI and HTTP; `watch` polling renderer.
- Structured logging with correlation fields.

Tests: golden `explain` output for a warning entry, a suppressed trigger, a
pending candidacy, an unavailable fact, and a partial fact — on **both** models.

### Day 10 — Gate A

- **The live → export → replay conformance test.** This is the gate.
- Both models in CI; race tests; fuzz the event and model decoders.
- Profile the active-window query at `max_samples`.
- Remove unused abstractions; document the event and model contracts.

Do not begin cognition work with a failing Gate A.

## 5. Week 3 — bounded configured cognition

Outcome: an admitted trigger produces one validated Decision, and suppression is
auditable.

### Day 11 — admission and snapshot

- `transition` and `sustained` trigger evaluation.
- Cooldown and budget suppression with durable reasons (F-19); no
  provider-status-dependent admission rule.
- Cold-start previous state (F-11).
- Canonical snapshot with a non-droppable contract/Situation/evidence-ID core,
  prior two versions, up to three prior Decisions (F-03), up to 20 events including
  sanitized non-admitted evidence (F-15), candidacy timeline, truncation detail.

Tests: unmatched transitions do not admit; a repeat inside cooldown records
`suppressed_cooldown`; `sustained` fires at `min_interval` and not before; budget
exhaustion suppresses without stopping ingestion; sustained budget suppression
retries at exact release without a wake loop; snapshot event and byte caps;
episode insert is atomic with the Situation transition.

### Day 12 — fake model and Decision validator

- `ReasoningClient`; deterministic fake keyed by snapshot hash.
- Injected-schema validation; evidence pointer restricted to the explicit allowed
  event-ID set; confidence-pointer resolution.
- Accepted-Decision transaction.

Tests: valid response; malformed JSON; schema type, enum, length, and
unknown-field violations; hallucinated evidence ID; out-of-range confidence;
oversized output; duplicate completion attempt.

### Day 13 — HTTP model adapter

One configured JSON/HTTP adapter: fixed request and response mapping,
authorization header, context cancellation, 30-second timeout, 16 KiB cap, status
and error normalization, token and cost extraction, secret-safe logs.

`httptest` cases: success, timeout, connection close, `429`, `500`, oversized
body, invalid response body. Do not add a second provider.

### Day 14 — durable worker

Single worker; atomic pending→running claim; terminal states; startup recovery of
stale running work; `retry-episode` with a five-total-attempt cap (initial plus at
most four manual retries) (F-23).

Tests: restart before the call; restart after the claim; provider failure while
events continue; one Decision under concurrent wake-ups; retry preserves the
attempt count and refuses past the cap.

### Day 15 — Gate B

Connect the fake and real-provider paths; Decision API and CLI rendering;
cognition-ratio reporting; close Gate B on both models.

## 6. Week 4 — correctness, lifecycle, hardening

### Days 16–17 — correctness suite

Committed traces with golden Situations and expected episode counts:

1. normal operation;
2. duplicate burst;
3. ID reuse conflict;
4. out-of-order correction inside the allowance;
5. late-beyond-allowance input;
6. out-of-contract value from a drifting sensor;
7. noisy threshold with `sustained_for` suppressing a spurious entry;
8. sustained warning triggering a `min_interval` re-diagnosis;
9. combined heat and vibration warning;
10. silence, then recovery;
11. window truncation producing `partial` facts;
12. crash and recovery.

Each trace runs against both models where applicable. Model tests use the fake.

### Day 18 — failure injection

Test-only crash points after event insert, after state update, after Situation
insert, after episode insert, before commit, and after commit. Verify the restart
result at every point. No partial committed mutation is allowed.

Start the 24-hour soak in the background at the end of this day.

### Day 19 — model lifecycle

- Reject in-place modification of an installed version.
- Install a changed version alongside the active one; `models diff`.
- `models compare` over one trace into isolated temporary databases, with the
  divergence report from
  [SITUATION_MODEL_DESIGN.md §9](SITUATION_MODEL_DESIGN.md) (F-21).
- `activate --mode=continue` refused on an incompatible pair; compatible
  activation keeps state, recomputes facts, clears candidacies, and writes a
  migration Situation;
  `--mode=reset --yes` writes `model_migration` Situations and supersedes open
  episodes (F-08).
- Verify every event and Situation retains its exact model digest.

### Day 20 — security, bounds, Gate C

- Loopback and bearer-token behavior; `expvar` off by default.
- Decoder fuzzing; log inspection for secrets and full prompts; file permissions.
- Enforce DB, request, snapshot, and response limits.
- Benchmark 100 events/second across 1,000 entities.
- Read the soak: memory, goroutines, file descriptors, database and WAL growth.
- Backup and restore.
- Close Gate C.

## 7. Week 5 — evaluation harness and release candidate

### Days 21–22 — scenario generator

`eval generate` with the seven fault labels and the perturbation matrix from
[EVALUATION_DESIGN.md §3](EVALUATION_DESIGN.md). Commit 70+ scenarios,
`expected_episode` labels, `labels.jsonl`, and the seed. Verify the suite
regenerates byte-for-byte.

### Day 23 — three-arm runner

`eval run` executing arm A once and arms B/C three times per scenario, with shared
context ceilings, token-parity checking between B and C, scenario-clustered run
metadata, per-scenario cost and latency capture, and blinded output export with a
seeded shuffle.

Arm B is built here. Build it to win: same schema, same budget, same objective,
stated thresholds. A strangled baseline invalidates the whole exercise.

### Day 24 — automatic scoring

`eval score`: suite-configured label/pointer extraction, evidence validity and
relevance, hallucinated-citation rate, overconfidence, urgency MAE and bias, cost,
clustered confidence intervals, and the mechanical verdict computation. The suite
manifest and Arm-A projection are committed before any scored run.

### Day 25 — release candidate

Versioned binary, both models, example traces, sample configuration, operator
runbook, benchmark and soak results. Run both demos from a clean checkout.

## 8. Week 6 — evaluation (not compressible)

- **Day 26** — pilot on 10 scenarios; fix harness defects only. Discard pilot
  scores; do not adjust the decision rule.
- **Days 27–28** — full run, all three arms, all scenarios; blinded human scoring
  by two reviewers.
- **Day 29** — compute automatic metrics, κ, bootstrap intervals, and the
  mechanical verdict. Assemble every case where arm B beat arm C.
- **Day 30** — write the evaluation report and hold the decision review.

## 9. Cut list

If the build runs late, cut in this order and record what was cut. Nothing below
line 6 may be cut without a new product decision, and nothing in Week 6 may be cut
at all.

1. `watch` — `explain` plus `situations` covers the need.
2. `models diff` — `models compare` is the one that finds real problems.
3. The `sustained` trigger type — transition triggers alone still test the
   hypothesis; the staleness gap (F-11) becomes a documented limitation.
4. `min` and `count` facts — reduces model expressiveness, does not change the
   architecture.
5. The 24-hour soak reduced to 4 hours plus a heap-profile leak check.
6. Correctness traces 8, 11, and 12 folded into others.
7. — *below this line, stop and re-plan instead of cutting* —
8. The cold room model (kills the injection invariant proof).
9. Exact wakes (kills replayability).
10. Non-admitted evidence retention (kills the sensor-failure scenarios).
11. Anything in the evaluation.

## 10. Pull request sequence

1. `build: bootstrap Go binary, deterministic testkit, and both example models`
2. `storage: add generic seven-table SQLite schema and migrations`
3. `models: validate and compile injected Situation Models`
4. `models: add bands, hysteresis proof, and output-schema allowlist`
5. `models: evaluate facts, conditions, candidacies, states, and traces`
6. `runtime: admit, deduplicate, and persist all well-formed events`
7. `replay: add virtual-clock JSONL runner, trace export, and golden tests`
8. `api: add loopback ingest and generic entity inspection`
9. `explain: render structured evaluation traces for operators`
10. `runtime: add exact age-crossing wakes and recovery checks`
11. `cognition: add trigger admission, suppression records, and frozen snapshots`
12. `cognition: validate Decisions against injected output schemas`
13. `provider: add one bounded HTTP model adapter`
14. `worker: add durable cognition processing and capped retry`
15. `models: add diff, compare, and activation compatibility modes`
16. `eval: add fault-injecting scenario generator`
17. `eval: add three-arm runner and automatic scoring`
18. `release: complete demos, soak, security, and runbook`

No PR may introduce a generic plugin, operator, graph, policy, broker, or worker
framework. No PR may add a code path, database column, or API route that exists
for one domain.

## 11. Required test layers

| Layer | Purpose |
|---|---|
| Unit tables | compiler passes, fact operators, band construction, state semantics, canonicalization |
| Property | the band property (§Gate A), `enter ⇒ stay` soundness, evaluator emits only configured states |
| Fuzz | model and event decoders |
| SQLite integration | model lifecycle, transactions, constraints, recovery, window queries at `max_samples` |
| HTTP integration | limits, status behavior, auth, provider failure |
| Golden replay | deterministic Situation and episode histories, both models |
| **Conformance** | live → export → replay hash equality; both models on one binary |
| Crash | atomicity at six boundaries; stale episode recovery |
| Benchmark and soak | query latency, memory, WAL, goroutine stability |
| Evaluation | harness determinism, token parity between arms B and C, seed reproducibility |

Unit, property, and integration tests run under the race detector in CI.

## 12. Definition of done

A work item is done only when:

- behavior **and** failure behavior are implemented;
- automated tests cover the boundary cases;
- errors carry stable machine-readable codes and useful context, without secrets;
- every queue, payload, query, window, and external call is bounded;
- race, vet, format, unit, and integration checks pass;
- both Situation Models exercise the path where applicable;
- user-visible contracts are documented;
- no excluded V0.1 capability was introduced indirectly.

## 13. Stop/go

The decision procedure is [EVALUATION_DESIGN.md §5](EVALUATION_DESIGN.md), and it
is pre-registered. It is not reopened after the results are seen.

The build phase produces a runtime. The evaluation phase produces the answer. Only
one of those is the point.
