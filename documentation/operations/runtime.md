# Runtime operations

Audience: local operators. Scope: start, serve, drain, kill, and verify the
current single-node runtime; deployment-specific qualification remains out of
scope.

## Start a bounded batch

For a supervised one-shot proof, use `run-live` with a fresh or intentionally
owned SQLite path:

```bash
./bin/agentic-stream run-live \
  --db runtime.db \
  --spec docs/design/examples/predictive-maintenance.situation.yaml \
  --trace examples/predictive-maintenance/testdata/trace-opening.jsonl
```

**Implemented and tested:** the runtime owns the pipeline, worker boundary,
policy/action path, and simulated effector for the batch.

## Start the continuous service

```bash
export AGENTIC_STREAM_SUBSCRIBER_TOKEN='rotate-out-of-band'
./bin/agentic-stream serve \
  --db runtime.db \
  --spec docs/design/examples/predictive-maintenance.situation.yaml \
  --trace examples/predictive-maintenance/testdata/trace-opening.jsonl \
  --listen 127.0.0.1:8080
```

**Implemented:** service ownership, loopback HTTP, readiness, metrics, SSE,
and optional JSONL polling. The source path must be append-only and the
deployment owns rotation/permissions.

## Readiness lifecycle

- `GET /health/live` reports process liveness.
- `GET /health/ready` reports whether the runtime can safely accept work.
- Readiness returns a problem response while recovery/ownership is not safe.
- Shutdown is bounded by the runtime's lifecycle context.

Do not use liveness as a readiness signal for traffic routing.

## Drain and kill

If `AGENTIC_STREAM_CONTROL_TOKEN` is configured, the current process exposes:

```bash
curl -X POST \
  -H "Authorization: $AGENTIC_STREAM_CONTROL_TOKEN" \
  http://127.0.0.1:8080/control/drain

curl -X POST \
  -H "Authorization: $AGENTIC_STREAM_CONTROL_TOKEN" \
  http://127.0.0.1:8080/control/kill
```

The handler compares the full `Authorization` header value to the configured
token. Drain refuses new admission. Kill also refuses later decisions from
the killed policy epoch. Treat these as operator actions and audit their use.

## Not a current surface

The design archive lists broader CRUD and inspection HTTP APIs. The current
handler exposes only the routes in the [HTTP reference](../reference/http-api.md).

## Next reads

- [HTTP reference](../reference/http-api.md)
- [Recovery](recovery.md)
- [Security hardening](security-hardening.md)
