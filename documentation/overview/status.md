# Current implementation status

This page describes the repository as it exists now. It is intentionally more
conservative than a roadmap: a checkmark means the code and focused tests
support the claim, not that every production deployment scenario is complete.

## Implemented in the current tree

| Area | Evidence |
| --- | --- |
| SituationSpec compilation, semantic validation, CEL restrictions, canonical digests | `internal/spec/`, `internal/spec/schema.json`, compiler tests |
| Normalized JSONL and simulator trace ingress | `internal/ingress/`, `examples/predictive-maintenance/testdata/` |
| SQLite WAL storage and migrations | `internal/storage/`, `migrations/` |
| Deduplication, event-time processing, watermarks, late correction, quarantine, gaps | `internal/eventlog/`, `internal/engine/`, `internal/operators/` |
| Immutable Situation versions and provenance | `internal/situations/`, `internal/engine/` |
| Deterministic cognition scheduling, debounce, cooldown, coalescing, reconsideration | `internal/cognition/` |
| Bounded episodes, budgets, cancellation, fencing, recovery, rebind | `internal/episodes/` |
| Decision/Intent schema and binding validation | `internal/decisions/`, `internal/contractsv1/` |
| Deterministic policy, approvals, interlocks, calibration and epoch controls | `internal/policy/`, `internal/interlock/`, `internal/storage/` |
| Idempotent outbox dispatch, simulated/watch effectors, unknown outcomes | `internal/actions/` |
| Native deterministic/OpenAI-compatible executor and Go EpisodeWorker boundary | `internal/executor/`, `internal/worker/`, `proto/` |
| Deterministic, recorded, shadow, and counterfactual replay modes | `internal/replay/` |
| Loopback HTTP, readiness, metrics, durable SSE notifications, drain/kill control | `internal/api/`, `internal/notify/`, `internal/telemetry/` |
| Predictive-maintenance fixtures and stream-plane path | `internal/runtime/pipeline_test.go`, `examples/` |

## Partial or operationally restricted

- The executable supports JSONL ingestion and a simulator adapter. Durable
  Kafka/NATS/MQTT connectors are not present.
- `serve` binds loopback by default and refuses non-loopback listeners without
  an authenticated deployment proxy. A deployment must provide that proxy and
  its operational controls.
- A simulated effector is the safe default proof surface. Concrete external
  effectors require an integration-specific implementation and review.
- Environment-level release evidence, long-running soak evidence, and a
  stable-release process are not implied by the green unit suite.
- `config effective` exists as a CLI placeholder and reports that it is not yet
  implemented; it is not a configuration introspection API.
- SituationSpec `policy` values are schema-visible, but current runtime
  catalog enforcement reduces policy behavior to the implemented approval
  path. Do not rely on `deny` or `simulate` semantics until dedicated runtime
  enforcement and tests exist.
- SituationSpec retention and telemetry sections are compiled metadata, not
  currently enforced retention or telemetry controls.

## Deliberately deferred

- Web UI, graph engine, multi-agent mesh, general workflow orchestration,
  Python workers, and direct production model credentials in the runtime.
- Distributed scale-out and broker adapters before single-node deterministic
  semantics are proven in the target workload.

The [limitations](limitations.md) page is the release-facing explanation; the
[roadmap](../roadmap.md) is the sequencing view.

## Evidence posture

The repository has extensive focused tests and end-to-end tests. They are
necessary evidence, not a blanket production certification. The dated
implementation audit and operations readiness notes remain in the working
archive. The audit is a historical snapshot and predates the current migration
count; code, tests, and the public status manifest are authoritative:

- [`docs/STREAM_IMPLEMENTATION_AUDIT_2026-08-12.md`](../../docs/STREAM_IMPLEMENTATION_AUDIT_2026-08-12.md)
- [`docs/design/OPERATIONS_READINESS.md`](../../docs/design/OPERATIONS_READINESS.md)
- [`docs/design/BUILD_COMPLETION_BAR.md`](../../docs/design/BUILD_COMPLETION_BAR.md)

## Next reads

- [Compatibility](compatibility.md)
- [Limitations](limitations.md)
- [Predictive-maintenance walkthrough](../guides/predictive-maintenance.md)
- [Quality and release gates](../governance/quality.md)
