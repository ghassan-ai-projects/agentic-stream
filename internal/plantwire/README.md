# Plant wire module

Plant wire hosts one plant wire session at a time and serves two existing
ports from it: readings go to the ingress append path, approved commands come
from the effect port (`actionport`). It is an adapter beside `internal/device`
and holds no device, domain or board knowledge: kinds come from commissioned
kind files and births, the event mapping from a site file outside the
repository, routes from the capability catalog. The design and its decisions
are in [the plant wire adapter page](../../documentation/design/plant-wire-adapter.md).

Read [the vocabulary](UBIQUITOUS_LANGUAGE.md) before changing rules.

| Layer | Responsibility | State |
| --- | --- | --- |
| Domain | Envelope and sequencing, the birth view, handshake admission, environment rules, commissioning by kind digest, host settings, the session conversation (STATE, HSYNC, rebirth, goodbye), the site mapping and directory, device births, data and deaths, readings to normalized events, the heartbeat on event time, commands in flight and their settlement from receipts and results, the origin rule, the device view (boot, origin, outputs and their latest readings), read-back expectations, verification watches and `HSYNC` command evidence | built |
| Wire | The vendored contract (embedded schemas, `DIGEST`, `VERSION`), record decoding with size limits and exact integers and the body of each record type, outgoing records checked against their schema, kind file and site file parsing (the site file schema is embedded) | built |
| Transport | One connection at a time over an injected listener, one record per line, 2 s write deadline | built |
| App | Kind commissioning, the site file, host supervisor and session loop, the ingest queue and feeder, the effector: catalog bound, device lookup, final authorization, origin check, one `DCMD` through the session loop, a mailbox per command; the device authority calls and clearing by readings; verification from readings | built |
| Facade | `New`, `CheckSettings`, `Run(ctx, emit)`, `Close`, `OwnsRoute`, `Dispatch`, `DispatchAuthorized`, `VerifyDeviceCommand`, `VerificationWindow` | built |

`serve --plant-wire-socket` is the only caller. It creates the listener with
the owner-only Unix socket helper of `internal/worker` (absolute path, 0700
directory, 0600 socket) and hands it to `New`. A session's reader never waits on
the host: records are read on their own goroutine and answered from one loop,
so a barrier is answered at once. The session's state is the pure
`domain.Conversation`; the app layer only moves lines and timers. Heartbeats
use the injected clock, so tests drive them with a virtual clock.

## Ingest

`serve` runs the host as an in-process feed: `runtime.Pipeline.RunFeed(ctx,
host.Run)`, and ingress admits each emitted envelope exactly as a live socket
line (tenant, envelope contract, registered event schema; a refused envelope is
quarantined). The session loop puts the events of each record on a bounded
queue (4096) that one feeder goroutine hands to the pipeline, so a slow batch
never delays a barrier or a heartbeat; a full queue ends the session (the node
reconnects) and an emit error stops the host.

What becomes an event is the site file (`--plant-wire-site`, kept outside the
repository; an example is
[`examples/real-world-sensor/testdata/plant-wire/`](../../examples/real-world-sensor/testdata/plant-wire/thermal-bench.site.json)).
It is checked at start against its embedded schema (`internal/wire/site.schema.json`)
and the commissioned kinds: every site kind is commissioned, every mapped metric
is declared by its kind, no reading or heartbeat has two rules, every device of
a kind that gives events names its entity, every wire quality has an event
quality and none but `good` maps to the event quality of `good`. Without a site
file the host serves sessions and gives no events.

- A `DBIRTH` registers a device whose kind the birth announced and no kind file
  quarantines, and whose kind is the one the site file names for it. A rebirth
  forgets every device; a `DDEATH` of the device's current boot removes it.
- A non-historical `DDATA` of a born device on its current boot gives one event
  per sample whose metric the site file maps: the value under the rule's field
  only when the wire quality is `good`, the mapped quality, `boot_id`, the
  record's `seq`, `device_mono_us` when sent and `schema_version`; a non-good
  sample also carries an envelope quality flag. The event id is `pw-` and 32 hex
  digits of the SHA-256 of origin, device, boot, metric and event time, so a
  re-sent sample deduplicates in the event log. Historical data is counted and
  not ingested.
- The heartbeat runs on event time: the session's time is the largest
  `NSYNC.world_time_ns`, `birth_time_ns` and sample time seen. A born device of
  a kind with a heartbeat rule gives one heartbeat event (quality of `good`) at
  that time when its last good sample is no older than `stale_after_ms` and
  `every_ms` has passed since its last heartbeat. The host clock is never read,
  so a silent link or a dead device stops the heartbeats and the spec's
  missing-heartbeat operator fires.
- Each session ends with a log of its events, historical records not ingested
  and dropped records.

## The vendored contract

`internal/wire/plantwire-contract/` is a byte-identical copy of streamsim's
`contracts/plant-wire/` folder. Never edit it by hand: run
`make plant-wire-sync PLANT_WIRE_CONTRACT_DIR=<path to the master folder>`.
The wire tests fail when the folder's recomputed digest differs from its
`DIGEST`, when `VERSION` differs from `domain.ContractVersion`, or when a
safety member stops being required. A contract change is synced to streamsim,
the gateway and this repository together, and the host accepts exactly one
contract version and digest.

The fixtures under `fixtures/` are the conformance suite: every `valid/`,
`must-ignore/` and `injection/` record decodes, every `invalid/` record is
refused at the member it names, and every exchange's opening handshake gets
the `STATE` it expects. Board mappings (`BOARD`, `rules/`) are the node's and
the gateway's concern; the host never reads one.

## Commands

With a capability catalog (`Config.Catalog`, from `serve --device-catalog`) the
host owns exactly the catalog's routes and `serve` routes them to it. The
effector runs on the dispatcher's goroutine: it bounds the command with the
catalog, finds the site device of its target, runs the final authorization
check, reads the device's boot and origin from the live session, refuses a
device on a hardware board unless the host has physical-actuation consent and
the board is in `--hardware-boards` (`ErrOriginNotAuthorized`, provider result
`refused: origin_not_authorized` with the board; a simulated board needs no
consent), and hands the order to the live session's loop, which stamps the `DCMD`
(the device's current boot, the host epoch, `safe_off`) and keeps a mailbox
for its answers. The conversation tracks the command (sent, queued, accepted)
and settles it from each receipt and result; when the session ends, a queued
command is refused `session_ending` and any other is unknown. The effector
waits until the terminal settlement, the deadline plus 2 s on the host clock,
or the dispatcher's call timeout. The outcome table is in
[the design](../../documentation/design/plant-wire-adapter.md#commands).

### Device authority

The host commands as the runtime owner (`Config.Authority`, `Config.OwnerEpoch`;
`serve` passes its own), through the same `internal/authority` operations as
device-wire:

1. The device's current boot is read from the live session and recorded
   (`RecordDeviceState` with `device_id` and `boot_id`). The first boot is
   clear, the same boot keeps its status and a new boot opens a reconciliation;
   while one is open the command is refused (`ErrReconciliationRequired`) and
   nothing is sent. Only devices the host commands are recorded.
2. `ClaimForDelivery` claims the target for that boot, binds the command (its
   digest is the content the node compares: device, command, args, expected
   boot; the binding also records the command's normalized target, so the
   dispatcher's reconciliation of an unknown outcome compares like with like)
   and asserts the claim. The session holds its claims and releases them
   when it ends.
3. The order carries the bound boot; if the device rebooted meanwhile the loop
   refuses it (`ErrBootChanged`).
4. After an executed command (succeeded or failed) the claim is asserted again;
   a lost claim makes the outcome unknown. An unknown outcome opens a
   reconciliation for the device boot (`RequireReconciliation`, on the priority
   path after authority loss).

While a reconciliation is open, step 1 presents the device's readings to the
authority's `ResolveWithReadings`: the latest sample the session holds of each
writable metric of the device's kind (the `DBIRTH` values count), with the host
time it arrived. The authority clears the reconciliation, recording the
readings as evidence, when every output has a good, live reading on the boot
newer than the latest opening (any reading of a rebooted boot qualifies), and
nothing bound to the boot is unresolved in the action ledger; otherwise the
command is refused. A reboot, or the switch from a simulated to a real session
of the same device, therefore clears on the new boot's first good reading, and
an unknown outcome on a reading that arrived after it, once the dispatcher has
reconciled the unknown command. Device-wire does not use it (decision D4).

### Verification

The conversation starts a watch when it settles a command as succeeded or
unknown: the expectations are the command's arguments named like a writable
metric of the device's kind, compared within the metric's `resolution` (a word
exactly). Every good, non-historical `DDATA` sample on the command's boot that
arrives afterwards is checked; when every expectation has matched, the
verification goes to the command's waiter and the numeric observations are
queued for the next `HSYNC` as `command_evidence` (at most 128 per barrier).
`VerifyDeviceCommand` waits for it until `VerificationWindow` (6 s: the
indicator twin reports every 5 s; only a matching reading verifies, so a
reading after the fan's 5 s lease lapsed cannot) after the dispatch returned, the call's context or
the session's end; then it withdraws the watch (`not_observed` evidence) and
gives no final status. A verified command reports succeeded with the readings
sealed as device-state evidence. A command with nothing to read back is
unverified at once. No `DQUERY` is sent.
