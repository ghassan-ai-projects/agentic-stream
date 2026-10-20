# Plant wire v1 — specification

- **Status**: contract v1.4.0, pre-release. Versioning rules apply from v1; compatibility is promised from the first field
  deployment.
- **Scope**: the session between a **node** (a simulated world, or an edge gateway in front of hardware) and a **host** (a plant
  supervisor). Field protocols below a gateway are out of scope; section 13 maps the one this contract meets first.
- **This folder is self-contained**: `SPEC.md`, `schemas/` (one JSON Schema per record, `KIND` for a kind file and `BOARD` for a
  board mapping), `fixtures/` (see `fixtures/README.md`), `DIGEST` and `VERSION`. It refers to nothing outside itself.

## 1. Pattern and roles

Devices announce themselves, then report; the host discovers rather than being configured. Semantics and record names follow
Sparkplug B, so a Sparkplug/MQTT binding remains possible; v1 is JSON Lines over one full-duplex stream, with no broker.
Receipts, results, state queries and the time barrier are extensions Sparkplug lacks.

| Role | Meaning |
|---|---|
| Node | One simulated world, or one edge gateway. The **node dials the host**. |
| Board | One physical controller. It owns the `boot_id`, the leases and the safe state; a reboot affects every device on it. |
| Device | One instance (`M1`, `pump-3`, `air_header`). Kind and id are immutable for a session. |
| Host | The supervisor. It owns permissions; births grant nothing. |

## 2. Security (v1)

v1 has **no authentication**. It is therefore carried only over a Unix socket that the host creates with owner-only permissions
(mode 0600, in a directory of mode 0700); a node refuses to dial a socket that is group- or world-accessible. TCP is not allowed
until mutual authentication is specified.

**Environment.** Every board in `NBIRTH.boards` carries `environment`, `simulated` or `hardware`, and the node carries one of
`simulated`, `hardware` or `mixed`. The node's value must agree with its boards: `simulated` means every board is `simulated`,
`hardware` means every board is `hardware`, `mixed` means at least one board of each. A schema cannot say that, so a host
checks it at the handshake. The host has its own setting, one of the same three values. A host set to `simulated` or `hardware`
accepts only a node of exactly that environment; a host set to `mixed` accepts all three. A node whose environment
contradicts its boards is refused even by a `mixed` host. Any violation is refused with `environment_mismatch` (section 6), and
the host records which rule failed. The **origin** of a device is the environment of its board (`DBIRTH` names the board);
nothing else may infer it, so a node that carries real and simulated devices announces each on a board whose environment is the
device's own. A node and a board only claim their environment: a host that needs the claim to be true verifies it by other means.

## 3. Envelope

Every record is one JSON object on one line:

| Member | Rule |
|---|---|
| `type` | The record type (section 4). |
| `protocol_version` | The version in force for the session (section 6). |
| `session_id` | Chosen by the node for one connection; deterministic for a simulated node (run id and connection count). |
| `seq` | One sequence **per direction** per session, from 0, by 1, without wrap. A writer assigns `seq` **when it transmits** the record, so the stream is always in `seq` order. |
| `lane` | `control` or `data`, fixed per type. A writer transmits queued control records before queued data; because `seq` is assigned at transmission, priority never creates a gap. |

**Versioning and unknown members.** A receiver ignores members it does not know in every record except `DCMD`, which is strict:
any unknown member makes it `malformed`, so a misspelt safety member can never be dropped silently. Enumerated values are
closed: a new value is a new protocol version. Integers are exact 64-bit (times in nanoseconds exceed 2^53); a receiver that
cannot parse them exactly must refuse the record.

## 4. Records

Node → host:

| Type | Lane | Carries |
|---|---|---|
| `NBIRTH` | control | `protocol_min`, `protocol_max`, `contract_version`, `contract_digest` (section 11), `environment`, `bd_seq`, `prior_last_seq` (last `seq` of the node's previous session, or -1), `capability_digest`, `kinds` (each entry with its `kind_digest`), `boards` (each `{board, environment, members}`), `node_metrics` |
| `DBIRTH` | data | `device`, `kind`, `board`, `boot_id`, `metadata_version`, `birth_time_ns`, `overrides`, the **current value** of every metric (`null` with quality `warming_up` before its first sample) |
| `DDATA` | data | `device`, `boot_id`, `historical`, `metrics`: one or more `{alias, value, quality, event_time_ns, time_source, uncertainty_ms?, device_mono_us?}` |
| `DDEATH` | control | `device`, `boot_id`, `reason` (`offline`, `link_down`, `retired`, `reboot`, `poll_failure`), `last_seq` |
| `NDEATH` | control | `bd_seq`, `reason` (`shutdown`, `connection_lost`) |
| `DRECEIPT` | control | `command_id`, `status`, `reason` (exactly when `status` is `refused`), `boot_id` |
| `DRESULT` | control | `command_id`, `reported_outcome` (`succeeded`, `failed`, `unknown`), `boot_id`; **self-reported**, it may lie |
| `DSTATE` | control | `device`, `boot_id`, `self_reported` (always `true`), `safe_state`, `outputs`, `values` |
| `NSYNC` | control | `through_seq`, `world_time_ns` (section 8) |

Host → node:

| Type | Lane | Carries |
|---|---|---|
| `STATE` | control | `host_id`, `online`, `heartbeat`, `heartbeat_interval_ms`, `authority_epoch`, `protocol_min`, `protocol_max`, `refusal` (section 6) |
| `NCMD` | control | `command`: `rebirth` |
| `DCMD` | control | `command_id`, `idempotency_key`, `device`, `command`, `args`, `expected_boot_id`, `deadline_ns`, `not_before_ns`, `authority_epoch`, `policy_digest`, `fallback_on_lease_loss` (`revert_to_local`, `hold_last`, `safe_off`) |
| `DQUERY` | control | `device` |
| `HSYNC` | control | `through_seq`, optional `command_evidence` |

Ids and names (devices, kinds, metrics, commands, boards, hosts) match `^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`: no spaces and no
prose, so an id cannot carry an instruction.

## 5. Births, metadata and commissioning

- `NBIRTH` declares each **kind once**: metrics (`data_type` in `number`, `integer`, `boolean`, `enum`, `string`; `unit` in UCUM;
  `eu_range`, optional `instrument_range`, `resolution`, `scan_class`, `deadband`, `description`, `writable`) and commands
  (argument schema, `risk_class`, `safe_stop`). `writable` means the metric is a setpoint some command assigns.
- `DBIRTH` names its kind and carries only instance values and **overrides**: `add` declares metrics whose names the kind does
  not have; `narrow` gives an `eu_range` inside the kind's. A host refuses a birth whose override widens, renames or adds
  commands. `birth_time_ns` dates the current values.
- **What a birth never says**: fault catalogue, onsets, affected states, expected effectors, deadlines, counterfactuals, effect
  models, failure-mode probabilities, latency distributions, true state, ground truth, profile weights. A simulated node
  sends no free-text description from its domain (domain descriptions are author notes) and strips descriptions from argument
  schemas.
- **Births are claims, not grants.** The host takes `risk_class`, `safe_stop` and argument bounds from its **commissioned
  inventory** (`discovered → reviewed → accepted`), never from a birth. A birth whose kind, id or command set differs from the
  commissioned one is quarantined and its commands denied. A changed `metadata_version` needs a new session and the host's
  acknowledgement before the host acts on the device.
- A board names its `environment`; a device born on it (`DBIRTH.board`) has that origin. Origin is never read from a device
  name, a kind, a metric or any other member.
- `capability_digest` is `sha256:` and the SHA-256 of the RFC 8785 canonical JSON of the array of `{kind, commands}` objects,
  sorted by `kind`, each `commands` array sorted by `name`, taken from `NBIRTH.kinds`.
- **Kind file and `kind_digest`.** A kind file is a JSON document that is exactly one `NBIRTH.kinds` entry (`kind`, `metrics`,
  `commands`) without its `kind_digest`; schema `KIND`. A kind file **never carries** a `kind_digest` (a stale one would be
  accepted and ignored, so the schema forbids the member), and the ranges of its metrics (`eu_range`, `instrument_range`) need
  `min` below `max`, which a validator checks beyond the schema. The `NBIRTH.kinds` entry schema is the `KIND` definition plus a
  required `kind_digest`, so the two cannot diverge. `kind_digest` is `sha256:` and the SHA-256 of the RFC 8785 canonical JSON of
  the entry without its `kind_digest` member. `capability_digest` is not enough to identify a model: it covers only
  `{kind, commands}` and ignores metrics, so two implementations that differ in metrics, units or ranges share it.
  The kind file is the contract. An implementation that has the file (a simulated domain) is proven equal to it by comparing
  its own entry with the file; a device is proven to match it only by announcing a `kind_digest` equal to the file's, which a
  host or gateway checks at commissioning. A host recomputes every `kind_digest` from the entry it received and refuses a
  session whose digest differs (`inventory_mismatch`).
- A kind speaks in the units and words that mean the same on every board; the raw values one board type uses for it live in
  that board type's **board mapping** (section 15), never in the kind file.

## 6. Handshake, sessions and rebirth

1. The node connects and sends `NBIRTH` with `protocol_version` 1 (the handshake envelope is always version 1).
2. The host answers `STATE`. If it accepts, `protocol_version` in `STATE` is the highest version within both ranges, and every
   later record in either direction uses it. If it refuses, it sends `STATE {online: false, refusal}` with `refusal` one of
   `version_unsupported`, `contract_unsupported` (a `contract_version` below the host's minimum, or a `contract_digest` it does
   not know at that version), `environment_mismatch` (the node's environment is not one the host accepts, or contradicts its
   boards; section 2), `inventory_mismatch` (a commissioned device missing from the birth, or a `kind_digest` that does not match its entry), `protocol_invalid` (`protocol_min` above
   `protocol_max`), then closes. A node does not reconnect after a refusal without an operator's action.
3. The node sends every `DBIRTH`, then data.

A **session** is one connection: a new connection has a new `session_id`, starts both sequences at 0, increments `bd_seq` and
reports the previous session's last `seq` in `prior_last_seq`. A **rebirth** (on `NCMD rebirth`) keeps the session and its
sequences: the node sends `NBIRTH` with the next `bd_seq`, then every `DBIRTH`. The host sends at most one `NCMD rebirth` until
the `NBIRTH` that answers it arrives.

## 7. Ordering, gaps, store-and-forward

- A receiver processes records in `seq` order. A record whose `seq` is ahead is held for at most 256 records or two heartbeat
  intervals; an exact duplicate (same `seq`, same content) is dropped. A gap that outlives the window makes the host send `NCMD
  rebirth` and makes the node end the session (it reconnects). Over an ordered stream (a Unix socket) nothing can arrive
  ahead, so a receiver may treat any gap as final and any repeated `seq` as a duplicate.
- A record that fails its schema still consumes its `seq`: a node answers an invalid `DCMD` that names a `command_id` with a
  `malformed` receipt and ignores any other invalid record.
- A late `NDEATH` whose `bd_seq` is older than the current birth is ignored. On socket loss the host synthesises `NDEATH`.
- **Store-and-forward**: data buffered while the host was unreachable is replayed with `historical: true`; the host never updates
  live state or fires real-time rules from it. Buffers are bounded by bytes and age; overflow drops the oldest and marks the next
  live metrics `stale`.
- A reboot is a `DDEATH` (`reboot`) for every device on the board, then each `DBIRTH` with the new `boot_id`.

## 8. Time

- **Time base.** A simulated node runs in **virtual time**: `deadline_ns`, `not_before_ns`, `event_time_ns` and heartbeats are
  world time. A hardware node runs in wall time (UTC), and its `uncertainty_ms` covers clock skew.
- **Barrier (quiescence).** A simulated world advances in slices. At the end of a slice the node sends `NSYNC {through_seq,
  world_time_ns}` and waits for `HSYNC` whose `through_seq` is at least the `NSYNC` record's own `seq` (so the barrier
  covers the `NSYNC` itself, and a late answer to an earlier slice never satisfies a later one). The node processes host records only at
  slice ends; a host record takes the world time of the slice end at which it is processed, so its effect is deterministic and
  recorded in the run's command log. The wall time waited is recorded as host latency. If no `HSYNC` arrives within the run's
  `barrier_wall_cap_ms`, the node records a missed barrier and advances; a slow host is never outrun silently.
- **Liveness.** The host sends `STATE` with an increasing `heartbeat` every `heartbeat_interval_ms` (wall time). A hardware
  node treats three missed heartbeats as host loss; a simulated node treats three consecutive missed barriers as host loss, and
  records it in the command log. On host loss the node stops executing commands, flushes its queue, lets leases lapse into their
  fallback and buffers data. `STATE {online: false}` is a graceful goodbye only.
- **Deterministic ids.** A simulated node derives `session_id`, `boot_id` and `bd_seq` from the run and its counters, never from
  the wall clock.

## 9. Commands

An optional HSYNC `command_evidence` array records a host's independent feedback decision. Each entry names
`command_id`, `device`, `boot_id`, `metric`, `expected`, nonnegative `tolerance`, and `outcome`
(`observed` or `not_observed`); `observed` requires `observed_value`. At most 128 entries are sent per barrier.
The reference host checks only a good, nonhistorical sample from the same boot, newer than the device's successful
result, against its explicit verification policy. A timeout is `not_observed`, not proof of failure. No evidence
means unknown. A receipt, device result or self-reported state is never independent confirmation. A relay forwards
`HSYNC` unchanged except `command_evidence`, which the relay consumes and does not pass to the gateway.

**Lifecycle.** Each `DCMD` receives at most one `DRECEIPT {status: queued}` and then exactly one terminal `DRECEIPT`
(`accepted` or `refused`); a terminal receipt supersedes a queued one. An accepted command receives exactly one `DRESULT`.
Commands still queued when a session ends get no receipt; the host counts them as refused (`session_ending`).

**Checks**, in this order, at receipt and **again at execution** for a queued command:

| Check | Refusal |
|---|---|
| Schema, known device, known command, argument bounds | `malformed`, `unknown_device`, `unknown_command`, `out_of_range` |
| `deadline_ns` not passed, not more than 24 h ahead; `not_before_ns` reached (else queued) | `expired`, `out_of_range` |
| `expected_boot_id` equals the board's current boot | `wrong_boot` |
| `authority_epoch` equals the **epoch floor**: the highest epoch any accepted `STATE` announced; a `STATE` with a lower epoch is ignored; a gateway persists the floor | `stale_epoch` |
| `policy_digest` is allowed (a bench allow-list; for a plant, an OT-owned limits envelope that host policy can only narrow) | `policy_not_allowed` |
| The host is alive | `session_ending` |
| Idempotency (below) | `duplicate` |
| The device's own preconditions, mode and interlocks | `local_control`, `permissive_not_met`, `locked_out`, `maintenance_isolated`, `interlocked`, `not_ready` |

**Idempotency.** A command's content is `{device, command, args, expected_boot_id}`; `idempotency_key` is `sha256:` and the
SHA-256 of its RFC 8785 canonical JSON. A node keeps, per key, the content digest, the terminal receipt and the result, for at
least 24 h plus transport delay (a gateway across restarts). A repeated key with the same content replays the stored receipt
and result with no second effect; with different content it is refused `duplicate`. A host that derives `idempotency_key` from
the content alone therefore gets a repeated identical command replayed and never executed again, so a host that may repeat a
command on purpose (for example to renew a lease) uses a fresh key per intent.

**Safe stop.** A command the node itself declares `safe_stop` is checked only for schema, device, command and boot; it skips the
deadline, epoch, policy and precondition checks, is never queued (`not_before_ns` does not apply) and is never refused for
idempotency (repeating it is harmless). A node refuses to start when one of its safe-stop commands has device preconditions
or an interlock.

**Repeats.** A repeat of a queued command (same key and content) is answered `queued` and receives the same terminal receipt and
result when the command runs once. A command whose acknowledgement was lost is not answered; a retry with the same key and
content receives the stored receipt and result. A node does not check that a key is the digest of its content: a host that
derives keys otherwise loses the protection.

**Leases.** `fallback_on_lease_loss` is required. A command whose arguments carry `lease_ms` falls back when the lease lapses
without renewal; every command falls back on host loss. A command whose board mapping declares a `lease` (section 15) carries
it in its raw arguments, so it falls back the same way although its kind arguments name no lease.

**Refusal classes.** A node says which class of refusal applies, never which fault caused it: a simulated node reports a failed
effector precondition as `permissive_not_met`, an interlock as `interlocked`, a random failure as `not_ready`. A precondition on a
hidden state therefore reveals one bit about it; domain authors put preconditions on observable states.

**Verification.** Receipt ≠ result ≠ observation ≠ verification: a host never counts a command done from a receipt or a
self-reported result.

## 10. Quality and truth

| Wire quality | Host envelope | Meaning |
|---|---|---|
| `good` | good | Valid measurement |
| `warming_up` | uncertain | Settling after start |
| `suspect` | uncertain | Plausibility check failed |
| `out_of_range` | bad | Outside `instrument_range` |
| `stale` | stale | Not refreshed within its scan class |
| `disconnected` | bad | Sensor link lost inside a live device |
| `manual_override` | uncertain | Value forced by a person |
| `out_of_service` | bad | Instrument taken out of service |

A non-`good` value never satisfies a control precondition. A single failed sensor inside a live device is a quality code, not a
`DDEATH`. A simulated node derives quality codes and death reasons only from declared observable models (channel availability,
perturbations, entity lifecycle), never from fault state, and no record member carries text about a fault.

Strings never carry instructions: descriptions (at most 200 characters) are for people; string metrics are bounded; a host quotes
every string it shows a model and never follows one.

## 11. The contract and its guards

- **Digest.** `DIGEST` holds `sha256:` and the SHA-256 of, for every schema in file-name order, the file name, a newline, the
  schema's RFC 8785 canonical JSON and a newline. It is the `contract_digest` a node announces. A schema change needs a reviewed
  `DIGEST` change. `VERSION` holds the contract version, the `contract_version` a node announces.
- **Safety-field floor.** Each repository tests that the members carrying safety and owner decisions stay required and their
  enumerations keep their safety values (commands, receipts, results, births, state, data).
- **Sync.** A repository vendors this whole folder byte-identically; its programs embed the schemas and never read a path at
  run time. A sync copies the folder from a configured master; release builds ignore that configuration.

## 12. Limits

A record is at most 64 KiB, except `NBIRTH` and `DBIRTH`, at most 1 MiB. A non-finite number (NaN, infinity) is never sent. A `DDATA` carries at most 500 metrics; an `NBIRTH`
declares at most 256 kinds and 4096 boards of at most 4096 devices.

## 13. Mapping to device-wire-v1 (for gateways)

| Plant wire | device-wire-v1 |
|---|---|
| `device`, `command`, `args` | `target`, `operation`, `parameters` |
| `deadline_ns` (absolute) | `expires_after_ms` = deadline − gateway now; refused `expired` when ≤ 0 |
| `not_before_ns` | `not_before_mono_us` on the device's monotonic clock |
| `expected_boot_id`, `idempotency_key`, `policy_digest` | same names |
| Refusals `malformed`, `out_of_range`, `expired`, `wrong_boot`, `duplicate`, `interlocked`, `not_ready` | same codes |
| `unknown_device`, `unknown_command` | `wrong_target`, `unknown_operation` |
| `DRESULT` `succeeded` | result `executed` |
| `DRESULT` `failed` | `rejected`, `expired`, `superseded` |
| `DRESULT` `unknown` | no result before the deadline |
| `DSTATE.safe_state` | result `safe_state`, or state query |
| `authority_epoch`, `stale_epoch`, `policy_not_allowed` | gateway only; device firmware is unchanged |

## 14. Version history

| Version | Change |
|---|---|
| 1.2.0 | Every board in `NBIRTH.boards` carries `environment` (`simulated` or `hardware`, required); `NBIRTH.environment` gains `mixed`; the host checks the node against its boards and against its own setting, which now has the same three values (section 2). |
| 1.3.0 | Optional HSYNC command_evidence carries explicit host feedback decisions, independently of board acknowledgement and self-report. |
| 1.4.0 | The board mapping file (`BOARD`, section 15) carries a board type's raw values, its lease and `board_digest`; the folder gains `VERSION`; section 9 says why a host that repeats a command on purpose needs a fresh `idempotency_key`. No record changes. |
| 1.1.0 | Every entry of `NBIRTH.kinds` carries a `kind_digest`; the kind file (`KIND`) is the contract of a kind (section 5). |

## 15. Board mapping

A **board mapping** file (`<board type>.board.json`, schema `BOARD`) says how one board type implements kinds: which raw
command and raw arguments a kind command becomes, and which kind metric a raw reading gives. A twin and a gateway read the same
file, so the twin and the real board cannot drift apart. A kind the mapping does not name passes through unchanged.

| Member | Rule |
|---|---|
| `board_type` | The board type, for example `arduino-thermal`. |
| `kinds.<kind>.commands.<command>` | `raw`: the raw command. `args`: each raw argument is `{value}` (fixed) or `{from, scale?}` (the kind argument `from` times `scale`, default 1). `by` with `table`: the kind argument `by` holds a word, and `table.<word>` gives raw argument values. `lease`: `{arg, ms}`, the raw argument that carries the board's lease and its fixed duration. |
| `kinds.<kind>.metrics.<metric>` | `raw`: the raw reading. Alone it is copied; `scale` divides it (the same direction as a command's `scale`: raw = kind value × scale); `table` maps a raw word to the kind's word. |
| `kinds.<kind>.raw_enums.<raw name>` | The words of a raw enumerated value in the order of their integer codes: a twin that holds the value as an integer uses the index. |

Three forms only (`value`, `from` with `scale`, `table`): no expressions. Every object with fixed members refuses unknown ones,
so a misspelt `lease` is never dropped silently. Beyond the
schema a validator checks (`fixtures/rules/`): every member name is an id (`board_name_invalid`); a string value of a raw name
listed in `raw_enums`, and every key of a metric `table`, is one of its words (`board_raw_word_unknown`); no raw argument is set
twice by `args`, a `table` row and the `lease` (`board_argument_conflict`); every `table` row sets the same raw arguments
(`board_table_uneven`); a metric read from the raw argument a command writes from that metric's name has the command's scale
(`board_scale_mismatch`). A host or gateway also checks the mapping against the kind files it serves: the same commands and
metrics, `from` and `by` naming kind arguments, table words equal to the kind's words.

**`board_digest`** is `sha256:` and the SHA-256 of the file's RFC 8785 canonical JSON. A board mapping never carries it (a
stale one would be accepted and ignored). Across the board mappings a node or gateway loads, each kind appears at most once.
