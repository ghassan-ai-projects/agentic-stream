# Plant wire adapter

Status: accepted design, built in rounds. The host sessions exist today
(`serve --plant-wire-socket`: handshake, commissioning, heartbeats, barriers,
one session after another), and so do the event mapping (site file, readings
and heartbeats to normalized events through ingress), the effector (one
`DCMD` per approved command, receipts and results into the outcome ledger),
the device authority calls (claim, bind, reconciliation barrier) as device-wire
makes them, the origin check per board, verification from the device's own
readings and the clearing of a reconciliation by readings. This page is the
decision record the module is built against.

## Context

Plant wire v1 is a session protocol between a **node** (a simulated world, or
an edge gateway in front of real boards) and a **host** (a plant supervisor).
Devices announce themselves in births, report readings, and take commands with
receipts and results. Agentic Stream already has two ports where such a
protocol plugs in: the ingress append path for evidence and the effect port
(`actionport`) for approved commands. It also already has one device adapter
behind the effect port, the device-wire link in `internal/device`.

## Decision

Add one module, `internal/plantwire`, that hosts one plant wire session at a
time and serves the two existing ports from it. It is an adapter beside
`internal/device`, not a replacement: nothing above the ports knows which
adapter is in use.

| Question | Answer |
| --- | --- |
| Responsibility | Host a plant wire session: handshake, commissioning, ordering, barriers and liveness; turn readings into normalized events; turn approved commands into `DCMD` records and their receipts and results into effects and verification. |
| Kind | Adapter module with an edge (socket I/O), shaped like `internal/device`. |
| Callers | `cmd/agentic-stream` only (`serve`). |
| Ports served | The ingress append path (readings become envelopes that ingress admits and quarantines as for any source) and `actionport.AuthorizedEffector` plus `actionport.DeviceStateVerifier` (commands). |
| Contract | The plant wire contract folder, vendored byte-identically from streamsim, pinned by version and digest. |

### Layers

| Package | Holds | Layer |
| --- | --- | --- |
| `internal/plantwire/internal/domain` | Pure rules: envelope and sequencing, record views, handshake admission, environment rules, commissioning by kind digest; later the site model, event mapping, heartbeat state machine, command plan, receipt classification and expected values. No I/O, no clock. | 13 |
| `internal/plantwire/internal/wire` | The embedded contract (schemas, `DIGEST`, `VERSION`), schema validation, record size limits, exact 64-bit integers, kind file parsing. | 14 |
| `internal/plantwire/internal/transport` | Framed connection over a listener `cmd` hands in, control lane before data, write deadline. | 15 |
| `internal/plantwire/internal/app` | Host supervisor and session, site file, ingest queue and feeder, effector with its `authority` calls, origin check, verification and clearing by readings. | 25 |
| `internal/plantwire` | Facade: configuration, settings check, `Run(ctx, emit)`, `Close`, the effect port operations and route ownership. | 26 |

The contract folder is
`internal/plantwire/internal/wire/plantwire-contract/` and `make plant-wire-sync`
copies it from `PLANT_WIRE_CONTRACT_DIR`. Tests fail when the folder's
recomputed digest differs from its `DIGEST`, or its `VERSION` from the version
the code implements.

### Session lifecycle

1. `cmd` creates the owner-only Unix socket and hands its listener to the
   module. A node dials the host; the host never dials. Sessions are served
   one at a time: a node that dials while a session is live waits until it
   ends.
2. `NBIRTH` is admitted or refused in a fixed order: `protocol_invalid`,
   `environment_mismatch`, `contract_unsupported` (exact contract version and
   digest), `version_unsupported`, `inventory_mismatch` (a kind digest that does
   not recompute, a kind announced twice, a required device missing). The host
   answers `STATE` and then sends a heartbeat every second. Every later
   `NBIRTH` of the session (the rebirth) is admitted by the same rules: a
   rebirth that contradicts them is refused with `STATE{online:false}` and ends
   the session, and a rebirth whose `bd_seq` does not advance is dropped.
3. A kind whose `kind_digest` differs from the commissioned kind file, or that
   no kind file commissions, is quarantined: its devices give no events and take
   no commands, and the session stays up.
4. Every `NSYNC` is answered with `HSYNC` as soon as the events read before it
   have left the ingest queue and the consumer has no reasoning in flight:
   an episode admitted and not yet terminal, or scheduler work that may start
   at once and has not yet been admitted (work held back by a debounce or
   cooldown window is not in flight). Reading never waits on the pipeline, but
   the barrier answer does, so a node that paces itself by barriers (an
   unpaced simulated world) cannot outrun the pipeline or the reasoning on its
   evidence; a real plant does not pace by barriers and is unaffected. The
   host polls the check, which `serve` wires to the episode ledger, only after
   the queue has drained, and the node's own barrier cap still bounds the
   wait. Commands are not part of the wait: an episode's command is dispatched
   only after the episode ends, and while an answer is held the host sends
   nothing else, so waiting on a command would stall until the cap. While an
   answer is held, the host sends nothing else (its records are sequenced) and
   keeps reading. A node that floods the queue without waiting for barriers
   still ends the session: the pipeline fell behind.
5. A sequence gap asks for one rebirth. A socket loss or `NDEATH` ends the
   session: claims are released, commands in flight are settled as the contract
   says, devices go offline. The host then accepts the next session without a
   restart. When `serve` stops, the host says goodbye (`STATE` offline).

### Site file

Which device feeds which entity, which reading becomes which event type and
field, which heartbeat event is emitted and how wire quality maps to event
quality is configuration, not code. It lives in one site file given at start
(`--plant-wire-site`), outside the repository; it is checked against its schema
and the commissioned kinds, and its digest is logged. No Go file in the module
names a metric, an event type or an entity type. The rules (registration on
births, readings, historical data, the heartbeat on event time) are in the
[module guide](../../internal/plantwire/README.md#ingest).

### Entity resolution

One seam answers both questions the adapter has: which entity a device reports
for, and which device an approved command for an entity and route reaches. It
is read from the site file now (the entity a device reports for); the catalog's
target bindings join it with the effector, its first caller. When the layout
arrives in births (a later contract change), a second implementation replaces
it without changing its callers.

### Consent per board

Each board in `NBIRTH` names its environment, `simulated` or `hardware`, and a
device's origin is the environment of its board, never a name. A command to a
device on a simulated board is sent freely. A command to a device on a hardware
board is sent only when the process runs the `physical` effect profile with
`--live-actuation` and `--owner-authorized`, and the board is named in
`--hardware-boards`; otherwise it is refused visibly with the board named. The
session can switch from simulated to real boards without restarting Agentic
Stream.

The rule is checked twice: once before the command is claimed, and once more in
the session that writes the record, from the device's board as the node
announced it at that moment. A node that moves a device to another board
between the two checks therefore cannot be commanded: the second check reads
the live origin, not the one the first check saw. The consent itself is one
fact of the `device` module (`device.GrantsActuationConsent`), which `cmd`
copies; `cmd` derives no rule of its own.

### Commands

The idempotency key of an approved command is forwarded unchanged (one key per
intent, so a repeated identical command really runs again). The deadline is
UTC, `authority_epoch` comes from `--plant-wire-epoch` (default: Unix seconds at
start), and every command falls back `safe_off` on lease or host loss (the
semantic kinds name no lease, so the host cannot tell a lease command; the
contract's fixtures and the reference host do the same). Receipts and
self-reported results are never counted as done: a command succeeds only when a
good, current reading of the commanded metric matches what was sent.

The catalog resolves an approved command (route, target binding, preset within
its bounds: `device.CapabilityCatalog.Bound`, shared with device-wire); the
directory gives the plant wire device of the catalog target; the device's
current boot comes from its birth in the live session. A command with no live
session, an unborn device or no site device is refused before anything is
sent. The answers map to the outcome ledger as follows.

| Node answers | Outcome |
| --- | --- |
| `accepted`, then `DRESULT succeeded` | pending verification; succeeded when the device's readings verify it, otherwise manual review |
| `refused` (any reason: `local_control`, `expired`, `stale_epoch`, `policy_not_allowed`, `wrong_boot`, ...), possibly after `queued` | failed, the reason in the provider result |
| `accepted`, then `DRESULT failed` | failed |
| `accepted`, then `DRESULT unknown`; `accepted` or a result on another boot | unknown outcome |
| still `queued` when the session ends | failed, `session_ending` |
| sent and unanswered when the session ends, or no terminal answer by the deadline plus 2 s, or the dispatch call ends | unknown outcome |

A command to a device on a hardware board without consent, or whose board is
not in `--hardware-boards`, is a known failure before anything is sent: the
error is `ErrOriginNotAuthorized` and the provider result says `refused:
origin_not_authorized` with the board and its environment.

Before a command is sent, the device's boot is recorded with the device
authority, a reconciliation (opened by a reboot or an earlier unknown outcome)
refuses it unless the device's readings clear it, and the target is claimed and
the command bound, through the same `authority` operations device-wire uses
(`ClaimForDelivery`, `RequireReconciliation`). An unknown outcome opens a
reconciliation; the session's claims are released when it ends.

The claim is renewed while the command waits for the node: a command held at
the node for the node operator (up to its route's expiry) outlives the claim
lease, and the lease is renewed from the same claim, so the target stays held
for the whole wait and the authority assertion after it still means something.
A renewal the authority refuses (another owner holds the target, or the
runtime lost its lease) makes the outcome unknown and opens the reconciliation.

The record is built and encoded before the command is registered as in flight:
a command the host cannot encode is reported as not sent (nothing left the
host), and the sequence number it took is given back, so no gap reaches the
node. A command whose write fails before a byte was written is reported the
same way; a write that failed after bytes were sent stays an unknown outcome.

### Verification

A command the node reports succeeded is never counted as done on its word.
Each argument whose name is a writable metric of the device's kind (the kind
file, never the birth) is read back on that metric: `set_cooling
{level_percent}` on `level_percent`, `show_state {state}` on `state`. The
reading must be a good, non-historical `DDATA` sample on the command's boot that
arrives after the `DRESULT` (the session starts watching when it settles the
result), and match within the metric's `resolution` (a word exactly), within a
**6 s window** from the dispatch's return: the slowest output in use (the
twin's indicator) reports every 5 s, plus a second of slack; the gateway reports
every 2 s. The window may outlast the fan's 5 s lease because only a matching
reading verifies: once the lease lapses the fan reads 0, which cannot verify
`level_percent 45`, so a verified fan command was read during its lease. Every expected metric must match; then
`VerifyDeviceCommand` reports succeeded with the readings sealed as
device-state evidence about the device output. The command binding records the
command's normalized target beside the output it was bound to, so the
dispatcher reconciles an unknown outcome from this evidence (the device
authority checks the command against the normalized target and the evidence
against the output). Without a match in the window, or with no argument to read
back, it gives no final status and the outcome stays in manual review. No
`DQUERY` is sent: a relay answers none for a real device. The next `HSYNC`
carries each numeric decision as `command_evidence` (`observed` with the value,
or `not_observed` when the window lapsed); a word has no number for the
contract's `expected`, so a `state` read-back carries none.

### Clearing a reconciliation by readings

Without a clearing path a reboot, or the switch from a simulated session to a
real one (a new boot of the same device), would block the device for good. The
rule lives once in the device authority (`ResolveWithReadings`); plant wire is
its only caller (device-wire keeps its own path, D4). When the boot is under
reconciliation, the host presents the latest reading it holds of each writable
metric of the device's kind (including the values the `DBIRTH` carries), with
the host time it received them. The authority clears the reconciliation as
succeeded, recording the readings as the resolution's evidence, when every
output has a good, live reading on the boot that is newer than the latest
opening; every reading of a boot is newer than the reboot that opened its
reconciliation, so a reboot clears on the new boot's first good reading, and
an unknown outcome on a reading received after it. As for any resolution, it
refuses while a command bound to the boot is still unresolved in the action
ledger, so an unknown command must be reconciled first (by its verification or
an operator).

## What the adapter never does

- It holds no device knowledge: kinds, metrics and commands come from the
  commissioned kind files and the births, routes from the capability catalog.
- It holds no thermal or other domain word: event types, entities and quality
  mapping come from the site file.
- It never sees or sends a raw board value: commands carry the kind's units and
  words, and the node or gateway translates them with its board mapping.
- It never trusts a birth for authority: risk, safe stops and argument bounds
  come from the commissioned inventory and the catalog.
- It never follows a string from the wire: descriptions and string values are
  data.

## Consequences

- `serve` logs the policy digest its commands carry at start (`spec loaded`,
  `policy_digest`), beside the commissioned kind digests, the site file digest
  and the epoch, so a session plan's or gateway's policy list can be filled in.

- The session ships behind `serve` flags; device-wire stays as it is and the
  two links are not used in one process yet.
- A dispatch held at the node longer than 0.9 of the dispatch lease becomes an
  unknown outcome, so `serve --dispatch-lease` sets that lease and, with a plant
  wire link and a catalog, `serve` refuses to start when a route's
  `expires_after_ms` does not end inside 0.9 of it, less the answer grace (2 s)
  and the verification window (6 s) the same call waits for. Device-wire links
  are not checked: a device-wire command is not held for approval at the
  device, and the check would refuse today's thermal catalog (its indicator
  route expires after 60 s, past the 54 s call of the default lease).
- A host with a device catalog needs a site file: the site's devices are the
  ones the node must announce (a missing one is an `inventory_mismatch`), and
  every route target and safe stop must be a device of the site file. A host
  without a catalog needs no site file and only feeds events.
- Historical (store-and-forward) readings are counted and logged, not ingested
  live.
- A reboot opens the same reconciliation barrier as device-wire; the device's
  readings clear it (above).
- Safe stop over plant wire, and the minimum-version rule for contracts, wait
  for a production need.

## Alternatives considered

- **Extend `internal/device`.** Rejected: device-wire is one serial exchange per
  command, plant wire is a multiplexed session with births, barriers and data.
- **Embed the site mapping like the simulator mapping.** Rejected: it would put
  a deployment's vocabulary into the binary.
- **Content-derived idempotency keys.** Rejected: a second identical command in
  the same boot would be replayed by the node and never run.

## Next reads

- [Decisions and actions](decisions-and-actions.md)
- [Stream processing](stream-processing.md)
- [ADRs and decisions](../adr/README.md)
- [Repository map](../architecture/repository-map.md)
