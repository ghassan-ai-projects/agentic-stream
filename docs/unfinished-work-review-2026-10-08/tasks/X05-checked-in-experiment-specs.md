# X05 — Check in the experiment's specs

Status: done · Decision: **complete** · Priority: P0 (experiment) · Size: S · Depends on: X01

Design and reasoning: [EXPERIMENT_DESIGN.md](../EXPERIMENT_DESIGN.md) G1, G4, G7, D3.

## Steps

1. Add `examples/real-world-sensor/zone-thermal-sim.situation.yaml`:
   zone-thermal with `executor.name: tamoz`, `dispatchPolicy: active`,
   `riskCeiling: R2`. This is the spec RUNBOOK-G1 builds by hand today.
2. Add `examples/real-world-sensor/zone-thermal-bench.situation.yaml`: inputs
   `temp`, `humidity`, `heartbeat` (the bench has no ambient or tach sensor yet).
   Open and transition on temperature mean and slope only, and keep
   `missing_heartbeat` in the trigger guard. Same intents, presets and risks as
   the sim spec. The header comment says why the ambient guard is absent and how
   to drive the thresholds on the bench.
3. `validate` also prints `policy_digest:` (from the policy facade's
   `DigestForVersion`), additive output, so the runbook reads the value for the
   gateway's `--device-policy-digest` instead of extracting it from an exported
   run. A README in the folder lists the run order and the X01 pins.
4. Extend X01: validate both specs, pin both digests, and run the X01 end-to-end
   scenario with the sim spec. Add a bench-shaped scenario: temperature and
   humidity only, entity `zone-01`, must open the Situation.

## Done when

The research runbooks reference these files instead of "copy and edit", and
X01 fails if either spec stops validating or changes digest.

## Result

- `examples/real-world-sensor/zone-thermal-sim.situation.yaml` and
  `zone-thermal-bench.situation.yaml`, with a README; the X01 end-to-end tests
  now run the sim spec.
- `validate` prints `policy_digest:` from the policy facade.
- Pins: spec digests (spec), policy digests (policy), thermal intent catalog
  digest for Tamoz parity (episodes, X04's part in this repository).
- `internal/replay/bench_spec_test.go`: on a temperature-and-heartbeat trace
  the bench spec opens and reaches `cooling`, while the sim spec never opens,
  which confirms G1.
