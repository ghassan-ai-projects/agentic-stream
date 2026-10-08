# X01 — Experiment compatibility guard

Status: done · Decision: **complete, before any other task** · Priority: P0 · Size: M

## Finding

The real-world-sensor experiment depends on eleven Agentic Stream surfaces
([EXPERIMENT_COMPATIBILITY.md](../EXPERIMENT_COMPATIBILITY.md) E1–E11). Some are
pinned piecemeal (the physical catalog digest in
`device/internal/domain/catalog_crossrepo_test.go`, the aquaculture intent
digest), but nothing runs the experiment's path end to end. That is how two
breaks (B1, B2) reached `main` unnoticed: a schema cleanup and a file move,
each reasonable on its own.

## Decision and reasoning

Add one hardware-free test, `TestRealWorldSensorExperimentPath`, in
`cmd/agentic-stream`, which runs the experiment's Agentic Stream slice the way
RUNBOOK-G1 does, with in-process stand-ins for the two other repositories:

1. Copy `docs/design/examples/zone-thermal.situation.yaml`, set
   `executor.name: tamoz` and `dispatchPolicy: active` (as the runbook does),
   and run `validate`.
2. Start `serve` with the runbook's flags: `--live-socket`,
   `--trace-format normalized`, `--worker-socket` and `--worker-name tamoz`
   (served by `workerfake` with **no** handshake features, like Tamoz),
   `--effect-profile emulator`, `--device-socket` (served by the device-wire v1
   UDS double already used in `device/internal/transport/uds_test.go`),
   `--device-catalog <canonical thermal catalog path>`, `--listen 127.0.0.1:0`.
3. Feed `zone.*` normalized envelopes on the live socket, shaped like
   `arduino_gateway.py` output, including one invalid line that must be
   quarantined.
4. Assert one Situation, episode, decision, intent, command, outcome,
   verification and device reconciliation, and that the device double received
   a command whose `policy_digest` is the pinned spec digest.
5. Run `export-run` and `verify-run`; assert the file set (E10) and
   `verdict: pass`.

Plus pin tests. Each one lives in the module that owns the surface (modularity
rule) and fails with a message naming the consumer repository to update:

| Pin | Owner test | Consumer |
| --- | --- | --- |
| zone-thermal compiled digest | `internal/spec/experiment_contract_test.go` | gateway `--device-policy-digest`, RUNBOOK-G1 |
| `zone.*` event schemas | `internal/spec/experiment_contract_test.go` | DHT11 mapping, Streams Simulator |
| thermal catalog path | `internal/contractsv1/experiment_contract_test.go` | RUNBOOK-G1 `--device-catalog` |
| thermal catalog digest | `internal/device/internal/domain/catalog_crossrepo_test.go` (existing) | bench firmware, Streams Simulator |
| policy document digest (the gateway allow-list value) | `internal/policy/experiment_contract_test.go` | gateway `--device-policy-digest` |
| `runtime-v1.proto` SHA-256 | `internal/contractsv1/experiment_contract_test.go` | Tamoz vendored copy |
| notification goldens SHA-256 | `internal/notify/experiment_contract_test.go` | Tamoz vendored copy |
| CLI commands and flags (E1) | `cmd/agentic-stream/experiment_contract_test.go` | runbooks |

Optional: when `REAL_WORLD_SENSOR_ROOT` is set (the same variable
`catalog_crossrepo_test.go` already uses), also compare against the files in the
research repository.

Why a Go test and not a script: it runs in `make ci-check` on every task, needs
no Ruby, Python or hardware, and fails at the commit that breaks the contract.
It does not replace the joined three-process run; it guards the Agentic Stream
side of it.

## Result

`cmd/agentic-stream/experiment_e2e_test.go` runs `serve` exactly as RUNBOOK-G1
does, with a Tamoz-shaped worker and a Streams-Simulator-shaped device that
speak only the public contracts (`experiment_standins_test.go`). It feeds the
committed thermal trace plus one malformed line, then checks the ledger, the
device command (`set_led` on `led-01` with the policy digest), and that
`export-run` and `verify-run` pass. It found X08 on its first run.

Being critical about the stand-ins: the worker stand-in must send a budget
update and terminal usage, or the runtime correctly refuses the attempt
(`budget_telemetry_missing`). Tamoz's `EpisodeStream` does both, and the
stand-in now mirrors that.

## Done when

- Both tests pass at `be37f6b` (after X02 updates the references) and are part of
  `make ci-check`.
- Each pin's failure message names the file in the other repository to update.
- `EXPERIMENT_COMPATIBILITY.md` links the tests.
