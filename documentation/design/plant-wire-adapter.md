# Plant wire adapter

Status: accepted design, built in rounds. The host sessions exist today
(`serve --plant-wire-socket`: handshake, commissioning, heartbeats, barriers,
one session after another), and so do the event mapping (site file, readings
and heartbeats to normalized events through ingress) and the effector (one
`DCMD` per approved command, receipts and results into the outcome ledger);
the device authority calls (claim, bind, reconciliation barrier) as device-wire
makes them; verification from readings and the origin check are planned.
This page is the decision record the module is built against.

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
| `internal/plantwire/internal/app` | Host supervisor and session, site file, ingest queue and feeder, effector with its `authority` calls (built); verification and origin (planned). | 25 |
| `internal/plantwire` | Facade: configuration, settings check, `Run(ctx, emit)`, `Close` (built); the effect port operations and route ownership (planned). | 26 |

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
   answers `STATE` and then sends a heartbeat every second.
3. A kind whose `kind_digest` differs from the commissioned kind file, or that
   no kind file commissions, is quarantined: its devices give no events and take
   no commands, and the session stays up.
4. Every `NSYNC` is answered at once with `HSYNC`; reading never waits on the
   pipeline, so a slow batch cannot miss a barrier.
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
| `accepted`, then `DRESULT succeeded` | pending verification (manual review until A3c verifies it) |
| `refused` (any reason: `local_control`, `expired`, `stale_epoch`, `policy_not_allowed`, `wrong_boot`, ...), possibly after `queued` | failed, the reason in the provider result |
| `accepted`, then `DRESULT failed` | failed |
| `accepted`, then `DRESULT unknown`; `accepted` or a result on another boot | unknown outcome |
| still `queued` when the session ends | failed, `session_ending` |
| sent and unanswered when the session ends, or no terminal answer by the deadline plus 2 s, or the dispatch call ends | unknown outcome |

Before a command is sent, the device's boot is recorded with the device
authority, a reconciliation (opened by a reboot or an earlier unknown outcome)
refuses it, and the target is claimed and the command bound, through the same
`authority` operations device-wire uses (`ClaimForDelivery`,
`RequireReconciliation`). An unknown outcome opens a reconciliation; the
session's claims are released when it ends. Clearing a reconciliation mirrors
device-wire (Q4): `ResolveReconciliation` with evidence, which no production
path calls yet.

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

- The session ships behind `serve` flags; device-wire stays as it is and the
  two links are not used in one process yet.
- A dispatch held at the node longer than 0.9 of the dispatch lease becomes an
  unknown outcome, so `serve --dispatch-lease` sets that lease and, with a plant
  wire link and a catalog, `serve` refuses to start when a route's
  `expires_after_ms` does not end inside 0.9 of it. Device-wire links are
  not checked: a device-wire command is not held for approval at the device,
  and the check would refuse today's thermal catalog (its indicator route
  expires after 60 s, past the 54 s call of the default lease).
- Historical (store-and-forward) readings are counted and logged, not ingested
  live.
- A reboot opens the same reconciliation barrier as device-wire.
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
