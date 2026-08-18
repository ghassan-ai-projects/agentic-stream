# CLI reference

The registered commands are defined in
[`cmd/agentic-stream/main.go`](../../cmd/agentic-stream/main.go). Use
`agentic-stream <command> --help` for Cobra's runtime-rendered help.

## `version`

Prints runtime version/commit metadata, contract version, and worker protocol
version.

## `validate <spec.yaml>`

Compiles and validates one SituationSpec.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--json` | `false` | Emit canonical JSON instead of the summary |

## `run --spec <spec.yaml> --trace <trace.jsonl>`

Runs deterministic replay against a fresh database and prints processed event
count, Situation-version count, and versions hash.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--spec` | required | SituationSpec YAML path |
| `--trace` | required | normalized JSONL trace path |
| `--db` | `<trace>.replay.db` | fresh replay database path |
| `--tenant` | `default` | runtime tenant |

Replay has no external effects.

## `run-live --spec <spec.yaml> --trace <trace.jsonl>`

Runs one owner-scoped live batch and prints pipeline counters.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--db` | required | SQLite runtime database |
| `--spec` | required | SituationSpec YAML |
| `--trace` | required | JSONL trace |
| `--tenant` | `default` | tenant |
| `--trace-format` | `normalized` | `normalized` or `simulator` |
| `--worker-socket` | empty | EpisodeWorker Unix socket |
| `--worker-name` | `native` | expected worker name |
| `--model-endpoint` | empty | OpenAI-compatible endpoint |
| `--model-name` | empty | model name; required with endpoint |
| `--worker-ca` | empty | worker CA PEM; enables mTLS |
| `--worker-cert` | empty | runtime client certificate |
| `--worker-key` | empty | runtime client private key |
| `--worker-server-name` | empty | expected worker certificate name |
| `--evidence-socket` | empty | runtime EvidenceTools Unix socket |
| `--evidence-key` | empty | hex HMAC key for EvidenceTools |

## `serve`

Starts the live runtime and HTTP readiness/observation surface.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--db` | required | SQLite runtime database |
| `--spec` | empty | spec for continuous ingestion |
| `--trace` | empty | append-only trace for continuous ingestion |
| `--trace-format` | `normalized` | `normalized` or `simulator` |
| `--tenant` | `default` | served tenant |
| `--listen` | `127.0.0.1:8080` | loopback HTTP address |
| `--owner-lease` | `1m` | runtime owner lease |
| `--poll-interval` | `1s` | continuous source polling |
| `--demo-mode` | `false` | admit fixtures; tests/demos only |
| `--model-endpoint` | empty | OpenAI-compatible endpoint |
| `--model-name` | empty | model name; required with endpoint |
| `--worker-socket` | empty | EpisodeWorker Unix socket |
| `--worker-name` | `native` | expected worker name |
| `--worker-ca` | empty | CA PEM file path; enables mTLS |
| `--worker-cert` | empty | runtime client certificate PEM file path |
| `--worker-key` | empty | runtime client private-key PEM file path |
| `--worker-server-name` | empty | expected worker certificate name |
| `--evidence-socket` | empty | EvidenceTools Unix socket |
| `--evidence-key` | empty | hex HMAC key for EvidenceTools |

`--spec` and `--trace` must be supplied together. A subscriber token is
required even when no SSE client is connected. Non-loopback listeners are
refused without an authenticated deployment proxy.

## `config effective`

Registered as a placeholder. It currently prints `config effective: not yet
implemented`; it is not a configuration API.

## Not registered yet

Design records may mention commands such as `init`, `ingest`, `simulate`,
`situation`, `episode`, `intent`, `explain`, `compare`, or `doctor`. They are not
current CLI commands and must not be used as implementation claims.

## Next reads

- [Configuration](configuration.md)
- [Quickstart](../getting-started/quickstart.md)
- [HTTP reference](http-api.md)
