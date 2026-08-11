# Agentic Stream V0.1 — Design Review

Reviewed: `design-v0.1` at commit `7202c31`.

Review standard: implementation readiness for a controlled prototype. The review
cross-checked prose, JSON contracts, SQL, examples, the executable hysteresis
oracle, the evaluation method, and the 30-day plan.

## Verdict

V0.1 has the right product boundary: one process, one durable deterministic plane,
one bounded cognition call, no actions, and domain behavior supplied by an
immutable Situation Model.

The baseline was not yet safe to implement literally. Several files described
different semantics for the same behavior, and four promises had no executable
path. Those blockers are resolved in the current revision.

The revised design is implementation-ready as a **time-boxed prototype**, with two
conditions:

1. Gate A must close before cognition work starts.
2. The 25 build days are a scope budget, not a delivery estimate. If the schedule
   slips, use the cut list; do not remove correctness tests or the evaluation.

## Resolved blockers

| ID | Finding | Why it mattered | Resolution |
|---|---|---|---|
| R-01 | Lateness was configured per window but persisted once per event | One event could be timely for one window and late for another, while storage had one `admitted` bit | One explicit model-level `lateness_allowance`; per-window admission is deferred |
| R-02 | `sustained_for` used event time in two contracts and evaluation time in another | Silence-driven candidacies could never complete under the event-horizon definition | Evaluation instant is normative everywhere |
| R-03 | Sustained triggers had no timer source | A six-hour review fired only after an unrelated event, or never | Sustained-trigger eligibility is a third exact-wake source |
| R-04 | Evidence-only string bands changed by value while prose said they never materialize | The material-change rule and the reference oracle disagreed | Every unreferenced fact has one `unreferenced` value band plus `unavailable` |
| R-05 | Snapshot truncation dropped sections from the bottom, where the output schema lived | An oversized snapshot could remove the contract needed to validate the request | Triggering Situation, episode contract, and allowed evidence IDs are non-droppable |
| R-06 | Rejected out-of-enum strings were retained and copied to the prompt | This reopened the free-text producer-to-model path the enum rule claimed to close | Raw text stays in storage; snapshots expose only type, length, digest, and rejection reason |
| R-07 | The hysteresis reference enumerated before checking its 100,000-combination cap | A hostile or accidental model could exhaust the compiler despite the documented bound | Cross-product size is checked before enumeration; regression tests cover it |
| R-08 | Dependent tables duplicated model ID, version, and digest without enforcing they named one row | Audit records could become internally inconsistent | Dependent records use the immutable digest as the normalized foreign key |
| R-09 | Retry prose allowed five manual retries while SQL allowed five total attempts | Operators and tests would implement different limits | Five total attempts: initial plus at most four manual retries |
| R-10 | Event-only trace export could not replay an upgrade or post-event silence | A history spanning versions or timer wakes violated the replay guarantee | Trace JSONL includes ordered model activations, immutable model documents, and an inclusive end horizon |
| R-11 | Arm B omitted evidence categories that arm C received | The evaluation could attribute an information advantage to structure | B and C share source categories and a token ceiling; the hypothesis is explicitly structured compression versus recency |
| R-12 | The decision rule overlapped and used a count as a percentage | The same result could satisfy Investigate and Stop | Ordered, exhaustive rules; hallucination is a citation rate; clustered confidence intervals are required |
| R-13 | Fact provenance could grow with every sample in every material Situation | A legal 100,000-sample window could make history unbounded | Situations store bounded provenance summaries; events remain the complete source |
| R-14 | Domain budget defaults were implicit, and open-episode suppression depended on provider latency | A supposedly deterministic rule depended on hidden defaults and asynchronous work | The hourly model budget is mandatory; provider status never participates in trigger admission |

## What is configurable and what is code

This is not a general-purpose rules engine. It is a small typed runtime whose
capabilities are code and whose product behavior is data.

| Injected Situation Model | Reviewed runtime code |
|---|---|
| event names and value contracts | strict decoding and type checking |
| units, enums, and lateness | exact comparison and admission mechanics |
| windows and sample caps | bounded window query implementation |
| facts and operator selection | implementations of `avg`, `max`, `min`, `count`, `latest`, `age` |
| states, priorities, thresholds, hysteresis, durations | generic state-selection algorithm |
| triggers, cooldowns, and budgets | generic admission and exact-wake scheduling |
| objectives, instructions, output schemas | bounded provider call and schema validation |

Adding a new domain changes only the left column. Adding a new operator changes the
right column and therefore requires a runtime release, tests, and a band-correctness
argument. That distinction is the product's configuration boundary; no domain
threshold or state transition is hardcoded.

## Remaining risks

### 1. Schedule confidence is low

Twenty-five build days is credible only as a strict prototype time box for one
experienced Go engineer already comfortable with SQLite and event-time semantics.
It is not a production delivery estimate. The compiler proof, crash testing,
activation/replay lifecycle, and evaluation harness each hide meaningful edge
cases.

The control is sequencing: Gate A after Day 10 proves the hardest deterministic
invariant. If it fails, do not hide the failure behind cognition work.

### 2. Recompute-per-event may miss the latency target

The design is bounded, not automatically fast. Group facts by `(input, window)` so
one ordered query feeds all aggregates for that group. Benchmark with the declared
sample caps before considering incremental state or a broker.

### 3. Synthetic evidence answers an architectural question only

The evaluation can show whether structured compression helps one reasoning model
on generated temporal faults. It cannot establish industrial diagnostic accuracy
or production safety. A Go verdict earns a field-data experiment, not deployment
authority.

### 4. Compatible activation is intentionally conservative

Continue mode preserves current state, recomputes facts, clears candidacies, and
writes a migration Situation. Changes to entity type, lateness, retained input
contracts, state IDs, or default state require reset. Expanding compatibility
before real upgrade examples exist would add risk without product evidence.

### 5. The HTTP surface is prototype-grade

Loopback binding and a static bearer token are adequate for controlled use. They
are not tenant isolation, identity, authorization policy, or internet-edge
security. V0.1 must not be presented as any of those.

## Review checks

The current artifacts pass:

- JSON parsing for all schemas and examples;
- Draft 2020-12 validation of both examples using the selected Go library, with
  external schema loading denied;
- compilation and fixture validation of local trace-schema references;
- both example models under the executable hysteresis oracle;
- regression tests for proof-budget enforcement and evidence-only string bands;
- application of the complete SQL schema to a fresh in-memory SQLite database.
