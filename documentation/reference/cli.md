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
| `--repeat` | `1` | replay N times in fresh databases and fail unless every Situation history hash is identical (cannot be combined with `--db`) |

Replay has no external effects. `--repeat 3` is the determinism check of the
release bar: three fresh runs must produce byte-identical Situation history.

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

Starts the live runtime with HTTP health checks and notifications.

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
| `--poll-interval` | `1s` | continuous source polling; with `--live-socket`, the pace of timers, due cognition and dispatch while the socket is quiet |
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
socket requires `--trace-format normalized`. Non-loopback `--listen` values are
always refused. For remote access, an authenticated deployment proxy must
forward to the loopback service.

`run-live` always consumes a trace and therefore rejects emulator and physical
profiles. `serve` can open a typed gateway link for those profiles only when
the catalog, firmware allow-list, and device socket are supplied, and it must
use `--live-socket` rather than `--trace`. The physical profile additionally
requires both explicit actuation and owner-authorization flags. Agentic Stream
never opens a raw serial port.

## Operator commands

Operator commands act on a runtime database (`--db`, `--tenant`, `--json`).
A command that changes runtime state first claims the runtime owner lease, so
it is refused while `serve` or `run-live` holds it: stop the runtime first.
Read-only commands take no lease. The interlock trip is the exception: it only
stops effects, so it works while the runtime runs or is hung.

### `interlock status | trip --reason <text> | clear --reason <text>`

The interlock is the global software stop for the action plane. Policy checks
it before creating a command and again immediately before delivering an
effect. `trip` blocks every effect and needs no lease; `clear` reopens the
action plane and needs the lease. Each change is versioned and records its
reason and time. It does not replace a physical e-stop.

### `quarantine list | release <event-id> | redrive <event-id>`

Invalid or not-yet-registered evidence is quarantined, never dropped. `list`
shows each record's status: `quarantined`, `released`, `redriven`, or
`rejected` when its retries ran out (the log records a gap for it). `release`
is the operator's decision to admit a record again; `redrive` re-validates the
released record against the schemas registered now and appends it to the log
exactly once, so the engine processes it on the next run. Both changes need
the runtime owner lease. A record that still fails validation stays released.

### `notifications prune --retention <duration> [--dry-run]`

Retires notifications older than the retention (minimum 168h) in one
transaction under the runtime owner lease, keeping tombstones so a retired
event identity is never accepted again and cursors stay monotonic. A client
whose cursor fell behind the retention gets `cursor_expired` and resnapshots.
`--dry-run` only counts. Schedule it with the host's own scheduler.

### `commands list | resolve <command-id> --status <status> --evidence <file>`

A provider timeout can mean the provider accepted the request, so the runtime
never retries such a command blindly: it waits in `outcome_unknown`,
`reconciling` or `manual_review`. `list` shows those commands. `resolve` closes
one as `succeeded`, `failed` or `manual_review` with independent evidence (a
JSON object with `source`, `evidence_type` and the fields of that type), records
the reconciliation outcome and its notification, and needs the runtime owner
lease. Device commands with device-state evidence are checked against their
device binding.

### `principals apply --file <principals.yaml> [--dry-run] | show`

Provisions the approval governance the `/v1/approvals` flow checks: relays,
approvers with their Ed25519 public keys, roles, role membership, and which
entity and risk (R0–R2) each role may approve. `apply` makes the tenant's
governance match the document in one transaction under the runtime owner
lease: principals the document omits are disabled, never deleted, so past
approvals keep their signers; memberships and authorities are replaced.
`--dry-run` reports the result and changes nothing. Unknown fields, repeated
ids, members without a key and R3/R4 authorities are refused.

## Not registered yet

Design records may mention commands such as `init`, `ingest`, `simulate`,
`situation`, `episode`, `intent`, `explain`, `compare`, or `doctor`. They are not
current CLI commands and must not be used as implementation claims.

## Next reads

- [Configuration](configuration.md)
- [Quickstart](../getting-started/quickstart.md)
- [HTTP reference](http-api.md)
