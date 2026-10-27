# CLI reference

The registered commands are defined in
[`cmd/agentic-stream/main.go`](../../cmd/agentic-stream/main.go). Use
`agentic-stream <command> --help` for Cobra's runtime-rendered help.

## Global flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `--log-level` | `info` | `debug`, `info`, `warn` or `error` |
| `--log-format` | `text` | `text` or `json` structured logs on stderr |
| `--event-schemas` | empty | directory of event-schema JSON files added to the built-in event schemas; read by `validate`, `run`, `run-live` and `serve`, accepted and ignored by every other command |

`--event-schemas` reads every `*.json` file of the directory, in name order. Each
file has the format of
[`event_schema_data.json`](../../internal/spec/internal/domain/event_schema_data.json):
a map from `<event_type>/<schema_version>` to the schema. A directory can add
schemas but never change one: a ref that is already registered must have an
identical definition. Every schema is checked as it loads: each field has a
plain name and `path` (a letter or underscore, then letters, digits or
underscores), a `type` of `number`, `integer`, `string` or `boolean` (empty
means `number`), an `enum` only on a `string` field, and the JSON Schema the
entry produces must compile. A file must declare at least one schema (`null`
and `{}` are refused) and may not repeat a key. Only regular files are read: a
symbolic link, directory or device named `*.json` is refused, as is a `*.JSON`
name; the directory holds at most 64 schema files of at most 1 MiB each. A
missing directory, a directory without `*.json` files or any refused file stops
the command before it opens a database or a listener. `run`, `run-live` and
`serve` log `event_schemas_digest` (and the directory) at start, so two runs can
be checked against the same schemas; `validate` uses the directory but does not
log the digest. Every other command (`explain`, `intent`, `quarantine`,
`operator` and the rest) accepts the root flag and ignores it.

The schemas are an input to compilation, not part of the spec digest or the run
artifact. Give the same directory every time a spec that uses them is compiled:
`serve` after a restart, and `run` when replaying the run (a fresh replay
database has no memory of the schemas the live run used). A runtime database
refuses a second definition of a ref it already stores, so a changed directory
fails at deployment rather than silently validating differently. Commands that
read a stored deployment instead of compiling the spec (`explain`) see only the
built-in schemas and do not need the directory.

`serve` logs one structured line when the spec is loaded (name, version,
digest), when the runtime is ready (owner epoch and lease), when it starts
serving (version, commit, listen address, database, tenant, effect profile,
worker socket), on every failed owner-lease renewal, when the lease is lost,
and when it stops and releases the lease. A failed renewal is retried on the
next heartbeat; readiness is cleared only once the lease has run out.

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
| `--source-db` | empty | recorded mode: a live runtime database, opened read-only, whose accepted decisions every replayed episode must match |
| `--worker-socket` | empty | shadow mode: candidate EpisodeWorker Unix socket, paired with the deterministic baseline on every replayed episode |
| `--worker-name` | `tamoz` | shadow mode: expected candidate worker name |
| `--json` | `false` | shadow mode: print the result, sealed comparisons and both decisions as JSON |

`--repeat`, `--source-db` and `--worker-socket` select different modes and
cannot be combined.

Replay has no external effects. `--source-db` verifies a live run without
calling a worker; `--worker-socket` compares a candidate worker with the
baseline and exits 0 when they disagree; see [replay and shadow modes](../design/replay-and-shadow.md). `--repeat 3` is the determinism check of the
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
| `--dispatch-lease` | `0` (the `1m` default lease) | lease of one approved command's dispatch: a device call still open after nine tenths of it is an unknown outcome. With `--plant-wire-socket` and `--device-catalog`, `serve` refuses to start when a route's `expires_after_ms` does not end before that call does |
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
| `--plant-wire-socket` | empty | owner-only Unix socket a plant wire node dials (absolute path in a 0700 directory); a continuous source, exclusive with `--trace` and `--device-socket` |
| `--plant-wire-environment` | `simulated` | plant wire nodes accepted: `simulated`, `hardware` or `mixed`; `hardware` and `mixed` need the physical profile with both consent flags |
| `--plant-wire-kinds` | empty | directory of the commissioned kind files (`*.kind.json`); required with the socket |
| `--plant-wire-site` | empty | site file kept outside the repository: checked against its schema and the commissioned kinds, its digest logged at start; without it no reading becomes an event, and with `--device-catalog` it is required (the site's devices are the ones the node must announce, and every route target must be one of them) |
| `--plant-wire-epoch` | `0` | authority epoch the host announces; `0` means Unix seconds at start |
| `--hardware-boards` | empty | repeatable hardware boards the plant wire host may command; needs a hardware or mixed host |

For continuous ingestion, supply `--spec` with `--trace`, or with
`--live-socket`, `--plant-wire-socket` or both. The live socket accepts normalized JSONL from reconnecting
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

`--plant-wire-socket` serves one plant wire session at a time (see
[the plant wire adapter](../design/plant-wire-adapter.md)): the host admits a
node's birth against the vendored contract and the commissioned kind files,
answers every barrier at once, sends a heartbeat every second, and accepts the
next session when one ends, without a restart. The readings and heartbeats the
site file maps become normalized events admitted like live socket lines (a
refused one is quarantined); historical (store-and-forward) data is counted,
not ingested. It counts as the effect link of
the `emulator` or `physical` profile instead of `--device-socket`. With
`--device-catalog` the host is the device effector of the catalog's routes:
an approved command goes to the live session as one `DCMD`, its receipts and
result become the outcome, and the device's own readings verify a success (a
success no reading confirms stays pending verification). A device on a
hardware board is commanded only with the physical profile's consent flags and
its board in `--hardware-boards`, and the board is read again from the live
session when the record is written, so a node that moves a device to another
board in between is refused. The target claim is renewed while a command
waits at the node, so a command held for the operator keeps its authority. At
start `serve` logs `spec loaded` with the
spec digest and the `policy_digest` its commands carry, the value a node's or
gateway's policy allow-list must hold (also printed by `validate`). Without a
catalog, commands fall back as with no device link (simulated under
`emulator`, refused under `physical`).

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

### `circuits list | close --rule <id> --version <n> --reason <text>`

A playbook rule's circuit opens when an audit disagrees, the rate cap is hit,
the device kind changes, a case is not verified or most matches fall outside
the cap; the rule then answers no case. `list` shows every circuit with its
cause. `close` switches the rule back on and records the reason; it needs the
lease, so stop the runtime first. A new approved version of a rule starts
with no circuit.

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

### `situation list [--entity <id>] | show <situation-id> [--version N]`

Read-only. `list` shows the tenant's Situations, newest evidence first, with
their current and last material version. `show` prints one version (default:
current) with its snapshot and evidence set.

### `explain situation <situation-id> [--version N] | trigger <trigger-id>`

Read-only answers to "why", from durable records only. `explain situation`
prints each field of the version with its value, the reducer that writes it
and the operator that computes the reducer's input (from the deployed spec),
then the evidence events the version was built from (log position, event id,
type, source, event time) and the trigger evaluations of that version.
Lineage is recorded per version, so evidence is the version's evidence set,
not a per-field attribution. `explain trigger` prints the evaluation (outcome,
score against threshold, lane, reasons and the delta it saw) and, when it
admitted work, the scheduler item, the episode it led to (which may have
assembled a later version) and any worker results the ledger refused.

### `episode show <episode-id>` and `intent show <intent-id>`

Read-only. `episode show` prints what the episode reasoned over (Situation
version, snapshot digest, executor), every fenced attempt with its terminal
record, every worker result the ledger refused, and each Decision with the
intents it proposed. `intent show` follows an intent through governance to its
effects: policy evaluations with reasons, approval requests and how they ended,
commands with their outcomes and verifications, and the watch a command
installed with its fires. There is no `intent approve`: approval is the signed
HTTP flow.

### `maintenance backup --output <file> | prune [--older-than 720h] [--vacuum]`

`backup` writes a consistent copy of the runtime database to a new file with
SQLite `VACUUM INTO`; it takes no lease, so it is safe while `serve` runs, and
it refuses an existing target. `prune` needs the lease (stop the runtime
first) and, in one transaction, removes bookkeeping older than `--older-than`
(default 30 days) that nothing references: ignored trigger evaluations no
scheduler item or reconsideration cites, applied engine inbox entries, fired
and cancelled timers, superseded Situation versions that no evaluation,
scheduler item, episode, decision or intent references (never the current,
last reasoned or last material version), and lineage sets no version cites.
The event log, decisions, intents, commands, outcomes and every admitted,
deferred, coalesced, rejected or expired evaluation are kept. `--vacuum`
reclaims the freed space afterwards.

## Not registered yet

Design records may mention commands such as `init`, `ingest`, `simulate`,
`compare`, or `doctor`. They are not
current CLI commands and must not be used as implementation claims.

## Next reads

- [Configuration](configuration.md)
- [Quickstart](../getting-started/quickstart.md)
- [HTTP reference](http-api.md)
