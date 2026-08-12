# Agentic Stream Build Completion Bar

This document turns the right-half build plan into an executable delivery bar.
It is repository-owned so that implementation and review do not depend on an
unversioned external checklist.

## Scope

The target is a single-node, production-capable Agentic Stream release with
integration readiness for an external, process-isolated episode worker. The
release includes the deterministic stream plane, bounded cognition, governed
effects, replay isolation, notifications, and operational evidence described
below. MQTT and broker-backed distribution remain later adapters unless a
separate release decision promotes them.

Environment-level production rehearsals are postponed for the current build by
explicit scope decision. They remain a future production-release gate, not a
current implementation blocker.

The existing worktree changes outside the implementation scope are preserved:
the research-document deletions and the pre-existing episode files are not
part of this build unless a phase explicitly adopts them.

## Non-negotiable gates

### Gate 0 — owned and reproducible baseline

- The external integration contract, lifecycle, protocol, and threat-model
  inputs used by code are copied or vendored under this repository with their
  source revision recorded.
- Canonicalization vectors and generated protocol artifacts have one pinned
  version and a drift check.
- Existing database status values have an explicit migration/backfill map.
- Every phase has a focused test command and a commit containing only its
  scoped files.

### Gate A — deterministic foundation

- Three fresh runs over each golden trace produce byte-identical canonical
  projections for accepted events, features, timers, watermarks, Situation
  versions, and trigger evaluations.
- Virtual time and deterministic IDs are used in expected-output tests; wall
  time cannot change a digest.
- RFC 8785 behavior is proven by all accepted vectors, native float cases, and
  all reject vectors, including duplicate keys, malformed Unicode, and the
  runtime's explicit negative-zero/unsafe-integer rejection policy. Every
  digest uses the mandatory `sha256:` prefix.
- Duplicate, bounded out-of-order, late correction, idle/rejoin, missing
  heartbeat, partition restart, and quarantine behavior are covered.
- Every published Situation field and trigger decision has durable provenance.

### Gate B — bounded cognition

- No event invokes a model directly.
- Every episode is bound to one immutable snapshot and has enforced deadline,
  model-call, input-token, output-token, tool-call, tool-result-byte,
  retry/repair, and cost limits.
- Episode aggregate, worker attempt, Decision validation, and outcome
  verification are separate durable state machines.
- Cancellation, supersession, coalescing, expiration, abandonment, and stale
  output are durable, explainable, and idempotent.
- Worker identity is `(episode_id, attempt_id, fence)`; same-snapshot retries
  cannot make an old attempt's Decision acceptable.
- Decision validation is fail-closed and duplicate delivery is idempotent.

### Gate C — safe effects

- Workers and models have no effector, credential, filesystem, shell, MCP, or
  mutable stream-spec capability.
- Every Intent is schema-validated and policy-revalidated immediately before
  dispatch, including freshness, preconditions, risk, approval, quota, and
  interlock state.
- Commands are created through an atomic outbox with stable idempotency keys.
- Fault injection at each dispatch boundary proves no duplicate accepted
  physical effect; unknown outcomes enter reconciliation and are never blindly
  retried.
- Replay modes cannot load production effectors, tokens, credentials, or
  outboxes. Counterfactual mode uses an explicit simulator only.

### Gate D — operational and integration release (postponed)

Gate D is retained as the future production-release bar. It is not required for
the current build completion decision.

- The worker protocol has a generated, runnable conformance suite, including
  handshake rejection, cancellation, capability scope, and trace propagation.
- Durable notifications support cursor resume, poison-event skip,
  slow-subscriber disconnect, cursor-expired audited resnapshot, and bounded
  retention.
- Upgrade, migration, backup/restore, corruption, disk-full, and
  unclean-shutdown tests pass.
- A defined soak workload records explicit memory, queue, timer, WAL, and
  database-growth thresholds and passes them.
- Security review evidence covers worker sockets, API binding, token
  algorithms/scope/expiry/rotation, secrets, event poisoning, and artifact
  retention. Residual risks are documented.
- One clean-checkout command runs the complete acceptance suite without
  silently skipping safety tests.

## Target end-state

After the bar is met, Agentic Stream is a Go modular monolith backed by SQLite
WAL. Its stable path is:

```text
ingress -> event log -> event-time engine -> Situation versions
  -> cognitive scheduler -> bounded episode -> typed Decision/Intent
  -> policy gateway -> outbox/effector -> outcome/reconciliation
```

The runtime can replay a trace deterministically, explain why a Situation or
cognitive opportunity changed, run a native executor in-process, or host an
external worker through the versioned worker port. External workers receive an
immutable snapshot and short-lived, capability-scoped read access; they return
typed evidence and Decisions but cannot execute effects.

The predictive-maintenance example is the release proof: repeated replay is
identical; bad, duplicate, late, and missing evidence are handled explicitly;
episodes are rare, bounded, cancelable, and fenced; accepted intents are
policy-governed and idempotent; uncertain effects reconcile; recorded, shadow,
and counterfactual replay are effect-safe; notifications and explanations are
durable; and operations have measurable recovery and soak evidence.

## Implementation loop

For each phase:

1. Freeze the phase contract and write the proving tests.
2. Implement the smallest design-conforming change.
3. Run focused tests, race tests where relevant, vet, formatting, and diff
   checks.
4. Have an independent implementation review inspect invariants, failure
   boundaries, migration safety, and test sufficiency.
5. Fix review findings and rerun the phase gate.
6. Commit only the phase files, record the evidence, and advance to the next
   phase.

The loop stops only when the relevant gate is green. A missing external
fixture, ambiguous contract, or unmeasured acceptance criterion is a blocker,
not a pass.
