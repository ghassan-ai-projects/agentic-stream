# Current implementation status

This page lists what the current code and focused tests support. An
implemented feature still needs evidence for its intended production
environment; the table is not deployment approval.

## Implemented in the current tree

| Area | Evidence |
| --- | --- |
| SituationSpec compilation, semantic validation, CEL restrictions, canonical digests | `internal/spec/`, `internal/spec/internal/domain/schema.json`, compiler tests |
| Normalized JSONL, simulator files, and live normalized JSONL Unix socket ingress | `internal/ingress/`, `examples/predictive-maintenance/testdata/` |
| SQLite WAL storage and migrations | `internal/storage/`, `migrations/` |
| Deduplication, event-time processing, watermarks, late correction, quarantine, gaps | `internal/eventlog/`, `internal/engine/`, `internal/operators/` |
| Immutable Situation versions and provenance | `internal/situations/`, `internal/engine/` |
| Deterministic cognition scheduling, debounce, cooldown, coalescing, reconsideration | `internal/cognition/` |
| Bounded episodes, budgets, cancellation, fencing, recovery, rebind | `internal/episodes/` |
| Decision/Intent schema and binding validation | `internal/decisions/`, `internal/contractsv1/` |
| Deterministic policy, approvals, interlocks and epoch controls | `internal/policy/`, `internal/interlock/`, `internal/storage/` |
| Idempotent outbox dispatch, simulated/watch effectors, unknown outcomes | `internal/actions/`, `internal/device/`, `internal/watch/` |
| Typed device-gateway integration with authority and reconciliation records; hardware qualification remains separate | `internal/device/`, `internal/authority/`, CLI effect-profile tests |
| Native deterministic/OpenAI-compatible executor and Go EpisodeWorker boundary | `internal/executor/`, `internal/worker/`, `proto/` |
| Deterministic (`run --repeat`), recorded (`run --source-db`) and shadow (`run --worker-socket`) replay from the CLI | `internal/replay/`, `cmd/agentic-stream/` |
| Operator commands: interlock, approval principals, quarantine redrive, notification retention, command reconciliation | `cmd/agentic-stream/`, [CLI reference](../reference/cli.md) |
| Read-only explainability: Situations, field derivations, trigger decisions, episodes and intents through to verification | `situation`, `explain`, `episode` and `intent` commands |
| Loopback HTTP, readiness, metrics, durable SSE notifications, drain/kill control | `internal/api/`, `internal/notify/`, `internal/telemetry/` |
| Predictive-maintenance fixtures and stream-plane path | `internal/runtime/pipeline_test.go`, `examples/` |

## Partial or operationally restricted

- The executable supports JSONL files, a simulator adapter, and live normalized
  JSONL over a Unix domain socket. Durable
  Kafka/NATS/MQTT connectors are not present.
- `serve` accepts only loopback listen addresses. For remote access, a deployment
  must provide an authenticated proxy that forwards to the loopback service,
  together with its operational controls.
- A simulated effector is the default for local tests and examples. Concrete external
  effectors require an integration-specific implementation and review.
- Environment-level release evidence, long-running soak evidence, and a
  stable-release process are not implied by the green unit suite.
- Inspection is read-only CLI over the runtime database; there are no HTTP read
  routes for Situations, triggers, episodes or intents.
- SituationSpec has no retention or telemetry blocks; runtime telemetry is
  configured at deployment time, and only notifications are pruned.

## Deliberately deferred

- Web UI, graph engine, multi-agent mesh, general workflow orchestration,
  Python workers, and direct model access to production effect credentials.
- Distributed scale-out and broker adapters before single-node deterministic
  semantics are proven in the target workload.

Read [limitations](limitations.md) for current boundaries and the
[roadmap](../roadmap.md) for the planned order of work.

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
