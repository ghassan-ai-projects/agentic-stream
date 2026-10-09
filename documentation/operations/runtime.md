# Runtime operations

Use this guide to start the local runtime, check readiness, and apply drain
or kill controls. Qualifying a production deployment requires separate
evidence for its configuration and environment.

## Start a bounded batch

For one supervised batch, use `run-live` with a new database or one you have
confirmed is owned by this runtime:

```bash
./bin/agentic-stream run-live \
  --db runtime.db \
  --spec examples/predictive-maintenance/predictive-maintenance.situation.yaml \
  --trace examples/predictive-maintenance/testdata/trace-opening.jsonl
```

**Implemented and tested:** the runtime owns the pipeline, worker boundary,
policy/action path, and simulated effector for the batch.

## Start the continuous service

```bash
export AGENTIC_STREAM_SUBSCRIBER_TOKEN='rotate-out-of-band'
./bin/agentic-stream serve \
  --db runtime.db \
  --spec examples/predictive-maintenance/predictive-maintenance.situation.yaml \
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

## Available routes

The design archive lists broader CRUD and inspection HTTP APIs. The current
handler exposes only the routes in the [HTTP reference](../reference/http-api.md).

## Metrics

`GET /metrics` serves Prometheus text sorted by name, with a `# TYPE` line per
metric and no tenant or event labels. Counters (`*_total`) count pipeline work,
batch failures, stale decisions, device frame errors, unknown outcomes,
verification-pending effects, lease expiries, safe stops, target-claim
rejections and live-socket lines. Gauges read at scrape time answer "are we
keeping up?":

| Gauge | Meaning | Alert when |
| --- | --- | --- |
| `agentic_stream_ingest_lag_events` | logged events the engine has not applied | it keeps growing |
| `agentic_stream_scheduler_items_pending` | reasoning opportunities waiting for admission | it keeps growing |
| `agentic_stream_outbox_open` | approved commands not yet delivered | it keeps growing |
| `agentic_stream_verifications_awaiting` / `_refuted` | effects awaiting or failing independent verification | refuted rises |
| `agentic_stream_apply_failures` | events set aside because applying them fails every time | above zero |
| `agentic_stream_database_bytes` | SQLite file size | near the disk budget |

The `dispatch_decision_p50/p95/p99_ns` gauges cover the most recent 1,024
episodes.

## Emergency stop

To stop every effect immediately, while the runtime keeps ingesting and
reasoning:

```bash
agentic-stream interlock trip --db runtime.db --reason "fan running hot"
```

Pending commands are refused from then on, before creation and before
delivery. After inspection, stop `serve` and reopen the action plane:

```bash
agentic-stream interlock clear --db runtime.db --reason "inspected wiring"
```

The software interlock complements, and never replaces, the physical e-stop
and the device's own safe state.

## Next reads

- [HTTP reference](../reference/http-api.md)
- [Recovery](recovery.md)
- [Security hardening](security-hardening.md)
