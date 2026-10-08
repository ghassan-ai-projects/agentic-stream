# X01 — Experiment compatibility guard

Status: todo · Decision: **complete, before any other task** · Priority: P0 · Size: M

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

Plus one table-driven pin test, `TestCrossRepositoryContractPins`, which fails
with a message naming the consumer repository when one of these changes:

| Pin | Consumer |
| --- | --- |
| zone-thermal compiled digest (as copied by the runbook) | gateway `--device-policy-digest` allow-list |
| thermal catalog digest and file path | Streams Simulator, gateway, RUNBOOK-G1 |
| `runtime-v1.proto` SHA-256 | Tamoz vendored copy |
| notification goldens SHA-256 | Tamoz vendored copy |
| `zone.*` event schema names and versions | DHT11 mapping file |
| CLI flag names in E1 | runbooks |

Optional: when `REAL_WORLD_SENSOR_ROOT` is set (the same variable
`catalog_crossrepo_test.go` already uses), also compare against the files in the
research repository.

Why a Go test and not a script: it runs in `make ci-check` on every task, needs
no Ruby, Python or hardware, and fails at the commit that breaks the contract.
It does not replace the joined three-process run; it guards the Agentic Stream
side of it.

## Done when

- Both tests pass at `be37f6b` (after X02 updates the references) and are part of
  `make ci-check`.
- Each pin's failure message names the file in the other repository to update.
- `EXPERIMENT_COMPATIBILITY.md` links the tests.
