# Limitations and release posture

Read this page before treating the repository as a production dependency. It
records the boundary between implemented behavior, deployment evidence, and
deliberate non-goals.

## Current release posture

The repository is an implementation-ready development snapshot. The runtime
has a working deterministic core, governed action path, worker protocol, local
HTTP API and Server-Sent Events (SSE), plus predictive-maintenance test data
for stream processing. Tests with simulated inputs cover later action stages. It is not yet a
stable release with a published compatibility promise or complete environment
qualification.

## Capability boundaries

### Ingress is local

The supported ingestion paths are normalized JSONL files, the simulator file
adapter, and live normalized JSONL over a Unix domain socket on `serve`.
Broker and MQTT integrations are deferred. If a deployment tails a file, it
must own file rotation, permissions, atomic append behavior, and source-health
monitoring.

### Effects are not exactly-once by assumption

The action plane uses stable identities, an outbox, leases, idempotency keys,
and durable outcomes. A provider timeout can mean that the provider accepted
the request; the runtime records an unknown outcome and does not blindly retry
it. An external integration must provide an idempotent or reconcilable route.
The CLI can select emulator/physical profiles through a typed device gateway,
with explicit catalog, firmware, and authority gates. That integration code
does not establish physical hardware qualification. Trace-backed runs remain
simulated-only.

### The local server is not an internet edge

`serve` accepts only loopback listen addresses. For remote access, an
authenticated deployment proxy must forward to that loopback service.
Notification subscriptions and operator controls require their respective
tokens. TLS termination, network policy, rotation, and rate limiting remain
deployment responsibilities.

### Model support is intentionally narrow

The native executor supports a deterministic provider and an OpenAI-compatible
structured-output adapter. The model can read scoped evidence and propose typed
outputs; it cannot access effectors, arbitrary shell, or production credentials.
External workers must implement the current-v1 Go protocol and pass the
conformance suite.

### Operational qualification remains separate from unit evidence

Focused tests cover deterministic replay, recovery, fencing, policy, actions,
notifications, and security boundaries. They do not replace workload-specific
capacity testing, storage backup rehearsal, key rotation rehearsal, or
long-running environment qualification. See the archived [operations
readiness](../../docs/design/OPERATIONS_READINESS.md).

### Some spec controls are not runtime controls yet

An intent's `policy` is `automatic` or `approval`; both are enforced, and
`automatic` never relaxes the risk route (R2 always needs approval, R3 and R4
are denied). An intent that must never run is simply not declared. Data retention
is an operational release concern and the SituationSpec intentionally does not
expose a retention or telemetry control until the runtime can enforce it.

### Domain state has a narrower implemented model

The current engine keeps a stable Situation/occurrence identity for the
tenant, deployment, partition, type, and entity; it does not automatically
create a new occurrence after resolution. Confidence starts at `1.0` without
a calibrated update mechanism. Hypotheses are Decision output, not Situation
state, so the trigger delta key `primary_hypothesis_changed` is true only for a
Situation's first reasoned version. Completeness is an evidence-processing status,
not a certified all-source coverage measure; the trigger `completeness` field
is not independently enforced by the current scheduler.

See [the domain model](../learn/domain-model.md),
[state initialization](../../internal/situations/internal/domain/situations.go),
[trigger state view](../../internal/cognition/internal/domain/delta.go), and
[trigger gates](../../internal/cognition/internal/domain/trigger_rules.go).

### Operator inspection and redrive are internal capabilities

Durable quarantine, approval, reconciliation, and explainability records exist
inside the runtime. Quarantine redrive, approval governance, the interlock and
unknown-outcome reconciliation have CLI commands; the public CLI and HTTP API do
not yet expose general inspection. A deployment needs approved internal tooling and
runbooks for those actions; they are not available as ready-to-use public operations.

## Deliberate non-goals

- No graph engine in the event hot path.
- No LLM invocation per event.
- No multi-agent orchestration or autonomous self-modification.
- No web UI, channel gateway, marketplace, or general workflow editor.
- No direct model-to-effector access.
- No distributed stream engine before the single-node contract is proven.

## How to use this page

When adding a capability, update this page and the status/roadmap pages in the
same change. Mark a capability implemented only after code, tests, operational
evidence, and user-facing reference agree.

## Next reads

- [Current status](status.md)
- [Security model](../architecture/security-model.md)
- [Security hardening](../operations/security-hardening.md)
- [Roadmap](../roadmap.md)
