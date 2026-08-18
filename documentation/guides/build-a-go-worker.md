# Build a Go EpisodeWorker

This guide describes the integration boundary, not a full worker framework.
Worker implementations are Go-only in the current version.

## 1. Read the source contract

Start with:

- [Worker protocol](../contracts/worker-protocol.md)
- [`docs/design/contracts/runtime-v1.proto`](../../docs/design/contracts/runtime-v1.proto)
- [`internal/executor/conformance/`](../../internal/executor/conformance/)

The runtime calls `Handshake`, then `Execute`. The worker streams events and
must emit exactly one terminal event for each attempt.

## 2. Preserve the authority boundary

The worker may inspect the immutable request and scoped evidence, call only the
EvidenceTools capabilities in its token, emit bounded model/tool telemetry,
and propose a schema-shaped Decision.

The worker may not execute an effector, access runtime credentials, mutate
Situation state, or widen its allowed Intent catalog/risk ceiling.

## 3. Preserve identity and budgets

Echo the request's episode ID, attempt ID, and fence on every event. Respect
deadline and budget updates. Treat cancellation as terminal for a
non-interactive episode; do not wait for human input.

## 4. Run conformance

Generated stubs are refreshed and checked with:

```bash
make proto-check
go test ./internal/executor/conformance ./internal/worker ./internal/episodes
```

The conformance suite exercises the fake, streamed, and separate-process
worker paths. A worker is not compatible because it merely speaks gRPC; it must
honor lifecycle, fencing, terminal, budget, and the currently enforced identity
and capability checks.

## 5. Configure the runtime

```bash
./bin/agentic-stream run-live \
  --db runtime.db \
  --spec docs/design/examples/predictive-maintenance.situation.yaml \
  --trace examples/predictive-maintenance/testdata/trace-opening.jsonl \
  --worker-socket /tmp/agentic-stream/episode-worker.sock \
  --worker-name example-worker
```

Worker and EvidenceTools Unix socket paths must be absolute and owned by the
deployment. Create the parent directory with restrictive permissions before
starting either process.

For mTLS, add the complete CA/certificate/key/server-name set. For reverse
EvidenceTools, add the private evidence socket and a 32-byte-or-longer hex
HMAC key. Keep socket permissions and key rotation in the deployment plan.

## Next reads

- [Worker boundary](../architecture/worker-boundary.md)
- [Security hardening](../operations/security-hardening.md)
- [Compatibility](../overview/compatibility.md)
