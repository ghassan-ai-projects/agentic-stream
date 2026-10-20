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
| **Barrier** | The node's end of a slice; the host answers at once through the `NSYNC`'s own sequence number. | `Conversation.Receive` | `NSYNC`, `HSYNC` |
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
| **Origin** | A device's environment: the environment of its board, never read from a name. | — | `DBIRTH.board` |
| **Hardware board** | A board the host may command as hardware, named at start. Needs a hardware or mixed host and physical-actuation consent. | `Settings.HardwareBoards` | `--hardware-boards` |

## Commissioning

| Term | Meaning | Code | Wire |
| --- | --- | --- | --- |
| **Kind** | A device model: metrics and commands in units and words that mean the same on every board. | `Kind` | `NBIRTH.kinds[]` |
| **Kind file** | One kind entry without its digest, given to the host at start. | `wire.ParseKindFile` | `KIND` schema |
| **Kind digest** | SHA-256 of the RFC 8785 form of a kind entry without `kind_digest`. | `KindDigest` | `kind_digest` |
| **Commissioned kind** | A kind file the host accepts; only its devices give events and take commands. | `Commission`, `Inventory` | — |
| **Quarantined kind** | An announced kind no kind file commissions, or whose digest differs. Its devices are ignored; the session stays. | `Inventory.Quarantined` | — |
