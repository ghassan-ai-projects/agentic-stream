# Reference-module migration status (2026-10-06)

Snapshot of every Go package after the migrations of this session. Companion
files: [DEADCODE.md](DEADCODE.md) (production reachability review),
[DEFERRED.md](DEFERRED.md) (behavior changes, deferred work, decisions to make),
[FOLLOW_UPS.md](FOLLOW_UPS.md) (prioritised work to do later)
and [deadcode-production-unreachable.txt](deadcode-production-unreachable.txt)
(raw `deadcode ./...` output).

Status legend: **Migrated** = facade, app, domain, store/adapter layers with
architecture gates and a dated record under `docs/`. **Decided: no** = reviewed
and deliberately left as is (reason given; see
[package-merge-review](../package-merge-review-2026-10-06.md)). **Candidate** =
fits the pattern, not yet done. **N/A** = pure rules, foundation or infrastructure.

## Migrated

| Package | Description | Record |
| --- | --- | --- |
| `authority` | Device claims, bindings, reconciliation and safety evidence | [authority](../authority-reference-module-2026-10-05/) |
| `device` | Device effect adapter and gateway session | [device](../device-reference-module-2026-10-05/) |
| `replay` | Effect-safe replay modes and verification | [replay](../replay-reference-module-2026-10-05/) |
| `eventlog` | Normalized event log, quarantine, gaps | [eventlog](../eventlog-reference-module-2026-10-05/) |
| `episodes` | Episode assembly and bounded execution | [episodes](../episodes-reference-module-2026-10-05/) |
| `evidence` | Capability-scoped evidence tools and call ledger | [evidence](../evidence-reference-module-2026-10-05/) |
| `decisions` | Pure Decision/Intent validator | [decisions](../decisions-reference-module-2026-10-05/) |
| `policy` | Policy plane and human approval workflow | [policy](../policy-reference-module-2026-10-05/) |
| `runtime` | Live pipeline, readiness and worker facades | [runtime](../runtime-reference-module-2026-10-05/) |
| `cognition` | Deterministic cognitive scheduler | [cognition](../cognition-reference-module-2026-10-05/) |
| `actions` | Governed dispatch plane, outcomes, reconciliation | [actions](../actions-reference-module-2026-10-05/) |
| `watch` | Derived-trigger watches installed as effects | [watch](../watch-reference-module-2026-10-06/) |
| `engine` | Deterministic stream engine, timers, Situation persistence | [engine](../engine-reference-module-2026-10-06/) |
| `ingress` | JSONL, simulator and live-socket sources, checkpoints | [ingress](../ingress-reference-module-2026-10-06/) |
| `approvalledger` | Human approval lifecycle, withdrawal of superseded approvals | [ledgers](../ledgers-reference-module-2026-10-06/) |
| `episodeledger` (with `scheduleledger`) | Scheduler queue, episodes, fenced attempts, rejection audit, recovery | [ledgers](../ledgers-reference-module-2026-10-06/) |
| `control` (with `costcontrol`) | Owner lease, epoch drain/kill, cost control, readiness gate | [control](../control-reference-module-2026-10-06/) |
| `notify` (with `notifycontract`) | Notification outbox, cursors, poison handling, lifecycle contract | [notify](../notify-reference-module-2026-10-06/) |

## Decided: no migration

| Package | Description | Reason |
| --- | --- | --- |
| `interlock` | Runtime interlock reader/writer | Tiny layer-0 leaf; merging into `control` widens dependencies |
| `admission` | Admits scheduler items into episodes | Thin orchestrator, no tables; merge blocked by layering |
| `api` | HTTP and SSE handlers | Thin adapter, no tables |

## Candidates (not yet reviewed in depth)

| Package | Description | Note |
| --- | --- | --- |
| `executor/native` | In-process episode executor, batch runner | 2.1k lines; adapter with artifact store; review shape |
| `executor/remote` | Remote worker executor over gRPC | 1.5k lines; adapter; review shape |
| `executor/fixture` | Fixture executor | Small; likely stays |
| `executor/conformance` | Executor conformance harness | Test-support; likely stays |
| `worker` | Reference worker server and UDS dialing | Mostly test-only reference; see DEADCODE.md |
| `runartifact` | Run export and verification artifacts | 1.2k lines; review shape |
| `soak` | Soak report computation | Small |

## N/A (pure rules, foundation, infrastructure)

| Package | Description |
| --- | --- |
| `actionport` | Approved-command and effector contracts |
| `canonicaljson` | RFC 8785 canonical JSON and digests |
| `clock` | Physical and virtual clocks |
| `contractsv1` | Versioned envelopes, schemas, digests |
| `duration` | Duration parsing |
| `eventschema` | Event schema registry and its embedded data |
| `ids` | Prefixed identity generators |
| `operators` | Deterministic stream operators |
| `situations` | Situation state machine |
| `spec` | SituationSpec compiler and deployment record |
| `storage` | SQLite WAL, migrations, retry |
| `telemetry` | Runtime counters and OpenTelemetry |
| `migrations` | SQLite migrations |
| `proto/agenticstream/runtime/v1` | Generated worker protocol |
| `cmd/agentic-stream` | CLI composition root |
