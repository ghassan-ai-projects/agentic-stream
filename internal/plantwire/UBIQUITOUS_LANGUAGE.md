# Ubiquitous language: plant wire

Plant wire is the session between a node and this host. The words below follow
the plant wire contract (`internal/wire/plantwire-contract/SPEC.md`); the
device-authority words (owner, device boot, claim, reconciliation) are those of
the [device-authority language](../authority/UBIQUITOUS_LANGUAGE.md).

## Session

| Term | Meaning | Code | Wire |
| --- | --- | --- | --- |
| **Node** | One simulated world or one edge gateway. It dials the host. | — | — |
| **Host** | This side: owns permissions; a birth grants nothing. | — | `STATE.host_id` |
| **Session** | One connection, from `NBIRTH` to socket loss or `NDEATH`. | `Envelope.SessionID` | `session_id` |
| **Record** | One JSON object on one line, validated against its schema. | `wire.Inbound` | `type` |
| **Envelope** | The members every record carries: type, version, session, sequence, lane. | `Envelope` | — |
| **Lane** | `control` or `data`, fixed per type; control is transmitted first. | `Envelope.Lane` | `lane` |
| **Sequence** | One counter per direction per session, assigned at transmission. | `Receiver` | `seq` |
| **Within** | A record after the handshake carries the session's id and the negotiated version. | `Envelope.Within` | — |
| **Gap** | A sequence number ahead of the next expected one; final on a Unix socket. The host asks one rebirth until the next birth. | `Gap` | `NCMD rebirth` |
| **Conversation** | The pure state of one session: what each node record makes the host answer, and when the session ends. | `Conversation` | — |
| **Heartbeat** | The host's `STATE` every second with an increasing counter, after the handshake. | `Conversation.Beat` | `STATE.heartbeat` |
| **Barrier** | The node's end of a slice; the host answers through the `NSYNC`'s own sequence number once the events read before it have left the ingest queue, so a node paced by barriers cannot outrun the pipeline. | `Conversation.Receive` | `NSYNC`, `HSYNC` |
| **Goodbye** | The host's offline `STATE` when it stops. | `Conversation.Goodbye` | `STATE.online: false` |
| **Authority epoch** | The integer the host announces in every `STATE`; from `--plant-wire-epoch`, or Unix seconds at start. | `Settings.Epoch` | `authority_epoch` |

## Handshake

| Term | Meaning | Code | Wire |
| --- | --- | --- | --- |
| **Birth** | The node's announcement of its versions, contract, environment, kinds and boards. | `Birth` | `NBIRTH` |
| **Admission** | The host's ordered checks of a birth; the first failure is the refusal, with a reason. | `Admission.Admit`, `Verdict` | `STATE.refusal` |
| **Schema refusal** | A record that fails its schema, with the members it fails at. It still consumes its sequence number. | `wire.SchemaError` | — |
| **Contract** | The vendored folder; a node must announce exactly its version and digest. | `ContractVersion`, `wire.ContractDigest` | `contract_version`, `contract_digest` |
| **Environment** | `simulated`, `hardware` or `mixed`: the node's claim, each board's claim, and the host's setting. | `CheckEnvironment` | `environment` |
| **Origin** | A device's environment: the environment of its board, never read from a name. | `Origin` | `DBIRTH.board`, `NBIRTH.boards[].environment` |
| **Origin check** | A device on a simulated board is commanded freely; on a hardware board only with physical-actuation consent and the board named; otherwise refused `origin_not_authorized`, nothing sent. | `Settings.Authorizes`, `ErrOriginNotAuthorized` | — |
| **Hardware board** | A board the host may command as hardware, named at start. Needs a hardware or mixed host and physical-actuation consent. | `Settings.HardwareBoards` | `--hardware-boards` |

## Commissioning

| Term | Meaning | Code | Wire |
| --- | --- | --- | --- |
| **Kind** | A device model: metrics and commands in units and words that mean the same on every board. | `Kind` | `NBIRTH.kinds[]` |
| **Kind file** | One kind entry without its digest, given to the host at start. | `wire.ParseKindFile` | `KIND` schema |
| **Kind digest** | SHA-256 of the RFC 8785 form of a kind entry without `kind_digest`. | `KindDigest` | `kind_digest` |
| **Commissioned kind** | A kind file the host accepts; only its devices give events and take commands. | `Commission`, `Inventory` | — |
| **Quarantined kind** | An announced kind no kind file commissions, or whose digest differs. Its devices are ignored; the session stays. | `Inventory.Quarantined` | — |

## Ingest

| Term | Meaning | Code | Wire |
| --- | --- | --- | --- |
| **Site file** | The deployment's mapping, outside the repository: devices to kinds and entities, readings to event types and fields, heartbeat rules, wire quality to event quality. | `Site`, `wire.ParseSiteFile` | — |
| **Mapping** | A site file checked against the commissioned kinds. | `Mapping`, `NewMapping` | — |
| **Directory** | The one seam that says which entity a device reports for (and, with the effector, which device a command reaches). | `Directory` | — |
| **Born device** | A device registered by its `DBIRTH` on its current boot until its death or a rebirth. | `liveDevice` | `DBIRTH` |
| **Reading** | A sample of a mapped metric that becomes one normalized event. | `Reading` | `DDATA.metrics[]` |
| **Historical data** | Store-and-forward samples; counted, never ingested live. | `DeviceData.Historical` | `historical: true` |
| **Session time** | The largest world time, birth time and sample time the session has seen; the heartbeat's clock. | `ingest.nowNS` | `NSYNC.world_time_ns`, `event_time_ns` |
| **Heartbeat event** | The liveness event of a born, fresh device, every `every_ms` of session time. | `HeartbeatRule` | — |
| **Fresh** | A device whose last good sample is no older than `stale_after_ms` of session time. | `liveDevice.fresh` | — |
| **Tally** | What a session gave: events, historical records not ingested, dropped records. | `Tally` | — |
| **Ingest queue** | The bounded queue between the session loop and the pipeline; full ends the session. | `enqueue`, `feedPipeline` | — |

## Commands

| Term | Meaning | Code | Wire |
| --- | --- | --- | --- |
| **Command order** | An approved command resolved for one device: id, idempotency key, policy digest, device, kind command and arguments, deadline. | `CommandOrder` | `DCMD` |
| **In flight** | A command sent in this session and not yet settled: sent, queued or accepted. | `inFlight` | — |
| **Settlement** | What an answer makes of a command in flight: queued, accepted (progress) or succeeded, refused, failed, unknown, not sent (terminal). | `Settlement` | `DRECEIPT`, `DRESULT` |
| **Abandon** | Settling every command in flight when the session ends: queued is refused `session_ending`, anything else unknown. | `Conversation.Abandon` | — |
| **Mailbox** | Where the session loop hands a command's settlements to the dispatcher waiting on it. | `session.mailboxes` | — |
| **Output** | A writable metric of a device's kind (the kind file, never the birth): what a command can set and a reading can confirm. | `Inventory.Outputs` | `KIND.metrics[].writable` |
| **Held reading** | The latest sample of a metric of a born device on its boot, with the host time it arrived; the `DBIRTH` values count. | `HeldReading` | — |
| **Device view** | A born device's boot, origin, outputs and the held reading of each. | `DeviceView`, `Conversation.Device` | — |
| **Expectation** | A command argument named like an output, to be read back on it within the metric's `resolution` (a word exactly). | `Expectation`, `Inventory.Expectations` | — |
| **Watch** | The session's wait for the readings that verify a command, started when it settles succeeded or unknown. | `watch`, `Conversation.Unwatch` | — |
| **Verification** | Every expectation matched by a good, live sample on the command's boot that arrived after its result. | `Verification` | — |
| **Verification window** | How long a command waits for its verification after the dispatch returns: 6 s, the slowest output's 5 s report period and a second of slack. | `VerificationWindow` | — |
| **Command evidence** | A numeric verification decision sent with the next barrier: observed with the value, or not observed when the window lapsed. | `Conversation.barrier` | `HSYNC.command_evidence` |
| **Clearing by readings** | Presenting a device's held readings to the device authority, which clears its reconciliation when each output has a good, live reading newer than the opening. | `clearByReadings`, `authority.ResolveWithReadings` | — |
