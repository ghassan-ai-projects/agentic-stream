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
| Domain | Envelope and sequencing, the birth view, handshake admission, environment rules, commissioning by kind digest, host settings, the session conversation (STATE, HSYNC, rebirth, goodbye) | built |
| Wire | The vendored contract (embedded schemas, `DIGEST`, `VERSION`), record decoding with size limits and exact integers, outgoing records checked against their schema, kind file parsing | built |
| Transport | One connection at a time over an injected listener, one record per line, 2 s write deadline | built |
| App | Kind commissioning, site digest, host supervisor and session loop (built); ingest feed, effector, authority calls (planned) | partly built |
| Facade | `New`, `CheckSettings`, `Run`, `Close` (built); the effect port operations (planned) | partly built |

`serve --plant-wire-socket` is the only caller. It creates the listener with
the owner-only Unix socket helper of `internal/worker` (absolute path, 0700
directory, 0600 socket) and hands it to `New`. A session's reader never waits on
the host: records are read on their own goroutine and answered from one loop,
so a barrier is answered at once. The session's state is the pure
`domain.Conversation`; the app layer only moves lines and timers. Heartbeats
use the injected clock, so tests drive them with a virtual clock.

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
