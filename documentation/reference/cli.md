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
| `--effect-profile` | `simulated` | `simulated`, `emulator`, or `physical`; trace-backed runs remain simulated-only |
| `--device-socket` | empty | typed device-gateway Unix socket for a non-simulated profile |
| `--device-catalog` | empty | closed capability-catalog JSON path for a non-simulated profile |
| `--device-firmware-digest` | empty | repeatable firmware allow-list for a non-simulated profile |
| `--live-actuation` | `false` | explicit physical-actuation gate |
| `--owner-authorized` | `false` | require explicit hardware-owner authorization for physical actuation |
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
| `--live-socket` | empty | live normalized JSONL Unix socket; mutually exclusive with `--trace` |
| `--trace-format` | `normalized` | `normalized` or `simulator` |
| `--effect-profile` | `simulated` | `simulated`, `emulator`, or `physical` |
| `--device-socket` | empty | typed device-gateway Unix socket for a non-simulated profile |
| `--device-catalog` | empty | closed capability-catalog JSON path for a non-simulated profile |
| `--device-firmware-digest` | empty | repeatable firmware allow-list for a non-simulated profile |
| `--live-actuation` | `false` | explicit physical-actuation gate |
| `--owner-authorized` | `false` | require explicit hardware-owner authorization for physical actuation |
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

For continuous ingestion, supply `--spec` with exactly one of `--trace` or
`--live-socket`. The live socket accepts normalized JSONL from reconnecting
clients and is the non-replay source for emulator and physical profiles. A
subscriber token is required even when no SSE client is connected. The live
socket requires `--trace-format normalized`. Non-loopback listeners are refused
without an authenticated deployment proxy.

`run-live` always consumes a trace and therefore rejects emulator and physical
profiles. `serve` can open a typed gateway link for those profiles only when
the catalog, firmware allow-list, and device socket are supplied, and it must
use `--live-socket` rather than `--trace`. The physical profile additionally
requires both explicit actuation and owner-authorization flags. Agentic Stream
never opens a raw serial port.

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
