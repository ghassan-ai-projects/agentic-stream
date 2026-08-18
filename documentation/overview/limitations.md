# Limitations and release posture

Read this page before treating the repository as a production dependency. It
records the boundary between implemented behavior, deployment evidence, and
deliberate non-goals.

## Current release posture

The repository is an implementation-ready development snapshot. The runtime
has a working deterministic core, governed action path, worker protocol, local
HTTP/SSE surface, and predictive-maintenance stream-plane fixture. Synthetic
focused tests cover later action stages. It is not yet a
stable release with a published compatibility promise or complete environment
qualification.

## Capability boundaries

### Ingress is local and file-oriented

The supported ingestion paths are normalized JSONL and the simulator adapter.
Broker and MQTT integrations are deferred. If a deployment tails a file, it
must own file rotation, permissions, atomic append behavior, and source-health
monitoring.

### Effects are not exactly-once by assumption

The action plane uses stable identities, an outbox, leases, idempotency keys,
and durable outcomes. A provider timeout can mean that the provider accepted
the request; the runtime records an unknown outcome and does not blindly retry
it. An external integration must provide an idempotent or reconcilable route.

### The local server is not an internet edge

`serve` is loopback-only by default and rejects non-loopback listeners without
an authenticated deployment proxy. Subscriber and control tokens are required
for the respective surfaces. TLS termination, network policy, rotation, and
rate limiting remain deployment responsibilities.

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

The schema accepts action policy values such as `automatic`, `approval`,
`deny`, and `simulate`, but the current catalog/runtime path does not provide
complete independent enforcement semantics for all four values. Retention and
telemetry sections are also compiled into the spec without active runtime
enforcement. Treat these fields as incomplete until implementation and focused
tests close the gap.

### Operator inspection and redrive are internal capabilities

Durable quarantine, approval, reconciliation, and explainability records exist
inside the runtime, but the current public CLI and HTTP surface does not expose
general inspection, approval resolution, unknown-outcome reconciliation, or
quarantine redrive commands. A deployment needs approved internal tooling and
runbooks for those actions; they are not turnkey public operations.

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
