# Plant wire adapter

Status: accepted design, built in rounds. Only the vendored contract, the
record codec and the handshake rules exist today (`internal/plantwire/internal/wire`
and `internal/plantwire/internal/domain`); the session, the event mapping and
the effector are planned. This page is the decision record the module is built
against.

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
| `internal/plantwire/internal/transport` (planned) | Framed connection over a listener `cmd` hands in, control lane before data, write deadline. | 15 |
| `internal/plantwire/internal/app` (planned) | Host supervisor, session, barrier, ingest feed, effector, calls to `authority`. | 25 |
| `internal/plantwire` (planned) | Facade: configuration, `Run`, the effect port operations, route ownership, close. | 26 |

The contract folder is
`internal/plantwire/internal/wire/plantwire-contract/` and `make plant-wire-sync`
copies it from `PLANT_WIRE_CONTRACT_DIR`. Tests fail when the folder's
recomputed digest differs from its `DIGEST`, or its `VERSION` from the version
the code implements.

### Session lifecycle

1. `cmd` creates the owner-only Unix socket and hands its listener to the
   module. A node dials the host; the host never dials.
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
   restart.

### Site file

Which device feeds which entity, which reading becomes which event type and
field, which heartbeat event is emitted and how wire quality maps to event
quality is configuration, not code. It lives in one site file given at start
(`--plant-wire-site`), outside the repository; its digest is logged. No Go file
in the module names a metric, an event type or an entity type.

### Entity resolution

One seam answers both questions the adapter has: which entity a device reports
for, and which device an approved command for an entity and route reaches. It
is read from the site file and the capability catalog now. When the layout
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
start), and a command that carries a lease falls back `safe_off`. Receipts and
self-reported results are never counted as done: a command succeeds only when a
good, current reading of the commanded metric matches what was sent.

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
  unknown outcome, so a `--dispatch-lease` flag and a start-time check against
  each route's expiry come with the effector.
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
