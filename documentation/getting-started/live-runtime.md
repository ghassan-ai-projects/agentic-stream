# Run the live runtime

Agentic Stream has two local runtime workflows:

- `run-live` processes one bounded trace batch and exits.
- `serve` owns a runtime lease, exposes health/metrics/SSE endpoints, and can
  repeatedly poll an append-only JSONL source.

## One bounded batch

```bash
./bin/agentic-stream run-live \
  --db runtime.live.db \
  --spec docs/design/examples/predictive-maintenance.situation.yaml \
  --trace examples/predictive-maintenance/testdata/trace-opening.jsonl
```

Use `--trace-format simulator` for streams-simulator `trace-record-v0.1`
adapter input. The default is `normalized`.

To use the native OpenAI-compatible adapter, provide both endpoint and model
name and keep the API key in the environment:

```bash
export AGENTIC_STREAM_MODEL_API_KEY='set-out-of-band'
./bin/agentic-stream run-live \
  --db runtime.live.db \
  --spec docs/design/examples/predictive-maintenance.situation.yaml \
  --trace examples/predictive-maintenance/testdata/trace-opening.jsonl \
  --model-endpoint 'https://model.example/v1/chat/completions' \
  --model-name 'model-id'
```

The runtime never puts the key in a trace, Situation, Decision, or command.

## Continuous JSONL serving

`serve` requires a subscriber token even when no client is connected:

```bash
export AGENTIC_STREAM_SUBSCRIBER_TOKEN='rotate-this-out-of-band'
./bin/agentic-stream serve \
  --db runtime.serve.db \
  --spec docs/design/examples/predictive-maintenance.situation.yaml \
  --trace examples/predictive-maintenance/testdata/trace-opening.jsonl \
  --listen 127.0.0.1:8080
```

When `--spec` and `--trace` are supplied together, the runtime polls the
append-only JSONL source at `--poll-interval` (one second by default), resumes
from durable connector state, and processes newly appended data. Supplying
only one of those flags is an error.

The server is loopback-only by default. A non-loopback address is refused
unless an authenticated deployment proxy is placed in front of it.

## Worker mode

Pass `--worker-socket` to delegate episodes to a current-v1 Go EpisodeWorker.
The native executor is not constructed on that route. Use the mTLS flags when
the worker connection is certificate-authenticated; `--worker-ca`,
`--worker-cert`, `--worker-key`, and `--worker-server-name` are all required
together.

An EvidenceTools reverse socket requires both `--evidence-socket` and a
32-byte-or-longer hex `--evidence-key`. The worker receives scoped,
short-lived capability tokens and read-only evidence access.

## Runtime controls

`serve` can expose authenticated `POST /control/drain` and
`POST /control/kill` for the current policy epoch when
`AGENTIC_STREAM_CONTROL_TOKEN` is set. Drain refuses new admission. Kill also
refuses later decisions from the killed epoch. The operator token is compared
as the `Authorization` header value by the current handler; follow the
[HTTP reference](../reference/http-api.md) exactly.

## Next reads

- [HTTP and SSE reference](../reference/http-api.md)
- [Operations](../operations/README.md)
- [Worker boundary](../architecture/worker-boundary.md)
