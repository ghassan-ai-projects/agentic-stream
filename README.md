# Agentic Stream

Agentic Stream is a streaming-native agent runtime. It continuously turns
unbounded evidence into durable, versioned **Situations**, then starts bounded
agent **episodes** only when a deterministic cognitive scheduler decides
reasoning is useful. Agents return typed Decisions and Action Intents; a
separate deterministic policy and action plane decides what may execute.

> Current posture: unreleased development snapshot. The runtime core and
> focused acceptance paths are implemented, but deployment qualification and a stable
> compatibility promise are not complete.

## Why it exists

Many event-driven systems either lose context in stateless alerts or invoke a
model for every event. Agentic Stream keeps the event-time stream deterministic,
publishes immutable Situation versions, and spends reasoning budget only on
bounded opportunities. The model proposes; policy disposes; the action plane
executes or records why it cannot.

## Product boundary

```text
evidence -> ingress/event log -> deterministic stream/operators
          -> immutable Situations -> cognitive scheduler -> bounded episode
          -> Decision/Intents -> policy -> idempotent action/outcome
```

The runtime deliberately has no graph engine in the event hot path, no LLM call
per event, no multi-agent mesh, no web UI, no direct model-to-effector access,
and no broker-backed distributed stream engine in version 1.

## Release-blocking invariants

1. Raw events are evidence, never executable instructions.
2. Event time, watermark, completeness, and late-data status are explicit.
3. Published Situation versions are immutable.
4. State changes are deterministic and serial per virtual partition.
5. Episodes bind one immutable snapshot and finite budget.
6. Models propose typed Intents but cannot execute effects.
7. Policy revalidates every Intent immediately before dispatch.
8. Cross-boundary work uses stable identities, durable inbox/outbox records, and
   idempotency.
9. Replay never performs external effects unless explicit simulation is selected.
10. Cognitive and action outcomes are explainable from durable records.

Read the [full invariant contract](documentation/architecture/invariants.md)
before evaluating an integration.

## Start here

```bash
make build
./bin/agentic-stream validate docs/design/examples/predictive-maintenance.situation.yaml
./bin/agentic-stream run \
  --db predictive-maintenance.replay.db \
  --spec docs/design/examples/predictive-maintenance.situation.yaml \
  --trace examples/predictive-maintenance/testdata/trace-opening.jsonl
```

Then follow the [quickstart](documentation/getting-started/quickstart.md) and
[predictive-maintenance walkthrough](documentation/guides/predictive-maintenance.md).

## Documentation

The curated public documentation is the primary entrypoint:

- [Documentation home](documentation/README.md)
- [Product overview](documentation/overview/product.md)
- [Current status](documentation/overview/status.md)
- [Limitations](documentation/overview/limitations.md)
- [Architecture](documentation/architecture/overview.md)
- [CLI reference](documentation/reference/cli.md)
- [HTTP/SSE reference](documentation/reference/http-api.md)
- [Contributing](CONTRIBUTING.md)
- [Security](SECURITY.md)
- [Support](SUPPORT.md)
- [Code of Conduct](CODE_OF_CONDUCT.md)
- [Changelog](CHANGELOG.md)

The `docs/` directory is the working archive: current design records,
machine-facing contract sources, research, audits, runbooks, and historical
iterations. See [`docs/README.md`](docs/README.md) before using it as a source.
The quickstart's `docs/design/examples/` paths are current test fixtures, not a
replacement for the curated public reading path.

## Technology

| Area | Choice |
| --- | --- |
| Runtime and CLI | Go 1.26.5 |
| Deployment | Modular monolith; optional Go EpisodeWorker processes |
| Persistence | SQLite 3 in WAL mode through `modernc.org/sqlite` |
| Authoring | YAML SituationSpec, JSON Schema, canonical JSON digest |
| Rules | Restricted deterministic CEL through `cel-go` |
| API | JSON health/metrics, authenticated SSE, operator controls |
| Worker protocol | Protobuf/gRPC over Unix domain socket; optional mTLS |
| Telemetry | OpenTelemetry traces and low-cardinality runtime metrics |
| Ingress | Normalized JSONL and simulator JSONL adapter |
| License | MIT |

## Development

```bash
make ci-check
make docs-check
go test ./...
go vet ./...
git diff --check
```

See [testing reference](documentation/reference/testing.md) and
[quality governance](documentation/governance/quality.md) for the full gate.

## License

MIT — see [LICENSE](LICENSE).
