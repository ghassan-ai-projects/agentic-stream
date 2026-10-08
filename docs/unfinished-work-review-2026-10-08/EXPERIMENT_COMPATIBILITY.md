# Real-world-sensor experiment compatibility

**Hard constraint (owner, 2026-10-08):** the real-world-sensor experiment
(`agent-research-lab/real-world-sensor`, with Streams Simulator and Tamoz) must
keep working after every task in this review. A task that would break it is
changed, not shipped.

This page records what the experiment uses from Agentic Stream, what is
already broken at `be37f6b`, and the impact of each task.

## What the experiment depends on

Collected from `assessment/RUNBOOK-G1.md`, `RUNBOOK-HIL-THIS-WEEK.md`,
`CURRENT-STATUS.md`, `runs/round-008-*/EXECUTION-REPORT.md`,
`assessment/tools/arduino_gateway.py`, the joined run
`.e2e-run/round-007-simulator-pass2` (its `stream.db` and artifact), and the
Tamoz `tamoz-stream` gem.

| # | Surface | Exact use |
| --- | --- | --- |
| E1 | CLI | `validate <spec>`; `serve --db --spec --live-socket --trace-format normalized --worker-socket --worker-name tamoz --effect-profile emulator\|physical --device-socket --device-catalog --device-firmware-digest --listen 127.0.0.1:0 --poll-interval` (plus `--live-actuation`, `--owner-authorized`, `--owner-lease`, `--tenant` on the bench); `run-live`; `export-run --db --tenant --output`; `verify-run <dir>` |
| E2 | Live ingress | Normalized JSONL envelopes from `arduino_gateway.py` over the `--live-socket` UDS; quarantine on invalid input |
| E3 | Event schemas | `zone.temp.observed/1.0`, `zone.humidity.observed/1.0`, `zone.heartbeat.observed`, `zone.ambient.observed`, `zone.fan_tach.observed` in `event_schema_data.json` |
| E4 | Spec | `docs/design/examples/zone-thermal.situation.yaml`, copied per run with `executor.name: tamoz` and `dispatchPolicy: active`. Uses sliding windows; `aggregate` (`mean`, `count`, `latest`), `slope`, `missing_heartbeat`; reducers `latest_event_time`, `set_union`; delta keys `novelty`, `phase_changed`, `severity_change`, **`primary_hypothesis_changed`**; intent policies `automatic` (R0, R1) and `approval` (R2) |
| E5 | Policy digest | The gateway allow-lists the Agentic **policy digest** (`--device-policy-digest`), which is the compiled spec digest. Any change to the canonical compiled form of zone-thermal changes it and stops device commands at the gateway |
| E6 | Device wire v1 | Agentic Stream as the UDS client of the gateway or emulator: command / receipt / result / state records, `policy_digest`, `capability_digest`, `safe_stop`, `query_state`, reconciliation |
| E7 | Capability catalog | Canonical thermal catalog JSON, digest `sha256:0d6122…fc44fc76` (equal to the board-reported `capability_digest`); the runbook passes its **file path** to `--device-catalog` |
| E8 | Worker protocol | `proto/agenticstream/runtime/v1/runtime-v1.proto`, vendored by Tamoz (`gems/tamoz-stream/contracts/runtime-v1.proto`); Tamoz advertises **no** handshake features |
| E9 | Notifications | SSE `/v1/events` with the notification contract; Tamoz vendors `notification-goldens-v1.json`; approval relay over `/v1/approvals/{id}` |
| E10 | Run artifact | `export-run` file set (`observations.jsonl` … `verdict.json`, `checksums.sha256`) and `verify-run` verdict |
| E11 | Tables read by hand | The runbook confirms one row each in situations, decisions, intents, commands, outcomes, verifications, device reconciliation, and counts `episodes` with `sqlite3` |

The joined run populated: `event_log`, `event_inbox`, `trigger_evaluations`,
`situation_versions`, `lineage_sets`, `scheduler_items`, `episodes`,
`episode_attempts`, `decisions`, `intents`, `policy_evaluations`,
`intent_dispatch_counts`, `commands`, `outbox`, `outcomes`, `verifications`,
`device_*`, `notifications`, `cost_*`, `runtime_interlock`, `runtime_owner`.

## Already broken at `be37f6b` (before any task here)

| ID | Break | Cause | Fix |
| --- | --- | --- | --- |
| B1 | The saved spec copy in `.e2e-run/round-007-simulator-pass2/zone-thermal-active.situation.yaml` fails `validate`: `field retention not found`, `field telemetry not found` | `1ebae6e` (2026-09-12) removed the unenforced `retention`/`telemetry` spec blocks, on purpose | No runtime change: the runbook copies the canonical spec, which no longer has the blocks. Delete the two blocks from any saved copy, and add a note to the runbook ([X02](tasks/X02-repair-experiment-references.md)) |
| B2 | `RUNBOOK-G1.md` passes `--device-catalog $STREAM/internal/contractsv1/conformance/v1/thermal-capability-catalog.json`; the file is now at `internal/contractsv1/internal/domain/conformance/v1/thermal-capability-catalog.json` | `98b84d6` (2026-10-06, contractsv1 refactor) moved it | Update the runbook, and pin every path other repositories reference with a test so a refactor cannot move them silently ([X02](tasks/X02-repair-experiment-references.md)) |

`CURRENT-STATUS.md` also cites `internal/actions/testdata/thermal_capability_catalog.json`,
`internal/actions/uds_transport.go` and `internal/actions/emulator_effector.go`.
They moved to `internal/device` in the ADR-017 refactor. These are documentation
references only; X02 updates them.

## Impact of each task

| Task | Touches | Impact | Required adjustment |
| --- | --- | --- | --- |
| U01 | intent `policy` enum | none: zone-thermal uses only `automatic` and `approval`; the compiled form of valid specs is unchanged, so E5 holds | X01 pins the zone-thermal digest |
| U02 | replay counterfactual | none | — |
| U03 | eventlog gap API | none: quarantine writes unchanged (E2) | — |
| U04 | engine run loop | none if golden hashes are equal | X01 must pass unchanged |
| U05 | native batch / artifact store | none: the experiment uses the Tamoz worker | — |
| U06 | reference worker server | none: the proto (E8) is not changed | **Do not edit `runtime-v1.proto`** |
| U07 | test-only exports | **risk to E7**: only the Go loaders move; the conformance JSON files stay at their current path | Stated in U07 |
| U08, U09 | constants, `config` command | none | — |
| U10 | `primary_hypothesis_changed` | **would break E4 and E5** as first written (compile rejection; the example spec edit changes the digest) | **Changed**: keep the key with its current behavior; see U10 |
| U11 | drop `replay_jobs`, `episode_events` | none: neither is in the joined run, the export or the runbook queries | X01 covers export |
| U12 | dead-code gate | none | — |
| U13 | operator lease rule | mutating operator commands are refused while `serve` runs | Provision principals before starting `serve`; interlock trip stays unfenced |
| U14 | interlock | none unless tripped; `/health/ready` must **not** fail on a tripped interlock (ingestion stays healthy) | Stated in U14 |
| U15 | principals | additive; table schemas unchanged, so existing manual SQL keeps working | Stated in U15 |
| U16–U18 | operator commands | additive | — |
| U19–U21 | replay CLI | additive; `run` without `--mode` is unchanged. U21's handshake offers no features, which matches Tamoz (E8) | — |
| U22, U23 | read-only commands | additive | — |
| U24 | calibration route | none: zone-thermal's R2 intent is `approval`, the route production already takes; the policy digest is the spec digest and does not change | — |
| P01 | design spec schema | none if the canonical example stays valid | X01 validates every example spec |

## Rules for every task

1. Run [X01](tasks/X01-experiment-compatibility-guard.md) before and after the
   change. It must pass unchanged.
2. Do not change E5–E10 bytes without a coordinated change in the other
   repositories: the zone-thermal compiled digest, the thermal catalog digest
   and path, `runtime-v1.proto`, the notification goldens, the device-wire
   records, and the run-artifact file set. X01 pins each one, and updating a pin
   needs a matching commit in the consumer repo.
3. CLI flags in E1 keep their names and meaning. New commands and flags are
   additive.
