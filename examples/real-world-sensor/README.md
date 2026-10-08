# Real-world-sensor experiment specs

The SituationSpecs the real-world-sensor experiment runs (the
`agent-research-lab/real-world-sensor` runbooks), checked in so every run uses
the same bytes and CI notices any change.

| Spec | Run | Sources |
| --- | --- | --- |
| [`zone-thermal-sim.situation.yaml`](zone-thermal-sim.situation.yaml) | RUNBOOK-G1, Streams Simulator | zone temperature, ambient, fan tach, heartbeat |
| [`zone-thermal-bench.situation.yaml`](zone-thermal-bench.situation.yaml) | RUNBOOK-HIL, Arduino bench | zone temperature, humidity, gateway heartbeat |

Both run Tamoz with active dispatch and an `R2` risk ceiling, and declare the
same intents: the watch fallback (R0), `set_indicator` (R1, automatic) and
`select_thermal_mode {mode: hold | bounded_cooling}` (R2, human approval). The
bench spec opens on the temperature mean and slope alone, because the bench has
no ambient sensor to discount a warming room.

Before a run, take the digests from `validate`:

```bash
agentic-stream validate examples/real-world-sensor/zone-thermal-bench.situation.yaml
```

`policy_digest` is the value the bench gateway allow-lists
(`arduino_gateway.py --device-policy-digest`).

## Approval governance

The fan intent (`select_thermal_mode`, R2) needs a signed human approval.
Provision the relay, the approver and their authority before starting `serve`
with [`principals.example.yaml`](principals.example.yaml) (replace the test
key):

```bash
agentic-stream principals apply --db runtime.db --file principals.yaml
```

## What CI pins

| Pin | Test |
| --- | --- |
| spec digests | `internal/spec/experiment_contract_test.go` |
| policy digests | `internal/policy/experiment_contract_test.go` |
| intent catalog digest (Tamoz parity) | `internal/episodes/internal/domain/thermal_catalog_test.go` |
| bench spec opens on temperature and heartbeat | `internal/replay/bench_spec_test.go` |
| closed loop through `serve` with the sim spec | `cmd/agentic-stream/experiment_e2e_test.go`, `experiment_live_feed_test.go` |

Changing a spec changes its digests: update the runbooks and the gateway
allow-list in the same change.
