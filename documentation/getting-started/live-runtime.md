# Run the live runtime

Agentic Stream has two local runtime workflows:

- `run-live` processes one bounded trace batch and exits.
- `serve` owns a runtime lease, exposes health, metrics, and Server-Sent Events (SSE) endpoints, and can
  continuously consume either an append-only JSONL source or live normalized
  JSONL over a Unix socket.

## One bounded batch

```bash
./bin/agentic-stream run-live \
  --db runtime.live.db \
  --spec examples/predictive-maintenance/predictive-maintenance.situation.yaml \
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
  --spec examples/predictive-maintenance/predictive-maintenance.situation.yaml \
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
  --spec examples/predictive-maintenance/predictive-maintenance.situation.yaml \
  --trace examples/predictive-maintenance/testdata/trace-opening.jsonl \
  --listen 127.0.0.1:8080
```

When `--spec` and `--trace` are supplied together, the runtime polls the
append-only JSONL source at `--poll-interval` (one second by default), resumes
from durable connector state, and processes newly appended data. Supplying
`--spec` with `--live-socket` instead starts a live normalized JSONL UDS source;
clients may disconnect and reconnect, and malformed lines are quarantined while
the source continues. With a live socket, `--poll-interval` also paces the work
that does not wait for new evidence (timers such as a missing heartbeat,
debounced cognition, approved commands), and episodes run beside ingestion, so
evidence keeps flowing while a worker reasons. `--trace` and `--live-socket`
are mutually exclusive.

For a live emulator-profile source, keep the effect boundary separate from the
telemetry socket:

```bash
./bin/agentic-stream serve \
  --db runtime.serve.db \
  --spec examples/thermal-chamber/zone-thermal.situation.yaml \
  --live-socket /tmp/agentic-stream-live.sock \
  --effect-profile emulator \
  --device-socket /tmp/device-gateway.sock \
  --device-catalog internal/contractsv1/internal/domain/conformance/v1/thermal-capability-catalog.json \
  --device-firmware-digest sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
```

Replace the example firmware digest with the allow-listed digest reported by
the device gateway.

For a spec with active dispatch, `serve` and `run-live` refuse to start unless
every declared intent has an owner: the built-in watch route, or a route the
device catalog names. With a device gateway (`emulator` or `physical`) nothing
falls back to a simulated or refused effect; in the `simulated` profile the
simulated effector owns every route. The error names the first intent without an
owner.

The live socket accepts normalized evidence. Device control uses a separate
socket. Receiving an event does not prove that an effect occurred; effects
need independent verification. Replay-file restrictions still apply.

The server accepts only loopback listen addresses. To expose remote access,
place an authenticated deployment proxy in front of the loopback service;
the proxy does not enable a non-loopback `--listen` value.

## Worker mode

Pass `--worker-socket` to delegate episodes to a current-v1 Go EpisodeWorker.
The native executor is not constructed on that route. Use the mTLS flags when
the worker connection is certificate-authenticated; `--worker-ca`,
`--worker-cert`, `--worker-key`, and `--worker-server-name` are all required
together.

An EvidenceTools reverse socket requires both `--evidence-socket` and a
32-byte-or-longer hex `--evidence-key`. The worker receives scoped,
short-lived capability tokens and read-only evidence access.

## Playbook

`--playbook <site>.playbook.yaml` loads a playbook of approved rules, compiled
against `--spec` at start (`agentic-stream validate <spec> --playbook <file>`
prints its digest; the spec and policy digests do not change). An active rule
that matches a case inside its approved cap answers it without a model call; shadow
rules are judged against the model's decisions. With a worker that announces
`draft_playbook_rule`, `--draft-after` verified, uncontradicted cases of one
key (three by default) ask it to draft a rule; the proposal is approved like an
intent, through the signed approval endpoint with its `rprop_` id, and the rule
is written to the playbook file in shadow (taking effect at the next start).
See the [playbook design](../design/playbook.md).

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
