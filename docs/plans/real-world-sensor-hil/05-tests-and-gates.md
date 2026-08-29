# Phase 05 — Validation matrix and gates

The program's validation matrix (`EXECUTION_PLAN.md`) mapped to concrete Agentic
Stream test locations. **Passing a lower level never substitutes for a higher
one.** Every production change ships with meaningful tests; no modified package
may show 0% coverage (AGENTS.md → *Definition of Done*).

| Level | Program requirement | Where it lives in this repo |
|---|---|---|
| unit | frame codec, schema, bounds, time mapping, policy materialization | `internal/contractsv1/*_test.go` (wire schemas), `internal/actions/serial_materialize_test.go` (bounds), `internal/eventschema/*_test.go` (observation schema) |
| property/fuzz | arbitrary fragmentation/corruption, dedup identities, state transitions | `internal/actions/serial_effector_test.go` (fuzz the codec + dedup by `idempotency_key`), `go test -fuzz` targets on the frame decoder |
| integration | dispatcher → emulator with unknown outcomes | `internal/actions/serial_effector_test.go` driving `Dispatcher.DispatchOnce` against an in-process emulator `DeviceTransport` |
| simulation | deterministic plant outcome + effector oracle | fixtures under `examples/thermal-chamber/testdata/`; oracle comparison is a Streams-Simulator concern consumed here as fixtures |
| HIL | real serial, reboot, power, sensor, actuator, feedback, e-stop | lab-executed (out of repo); Agentic Stream provides the effector + ledgers |
| soak | repeated faults, leak/resource checks, zero-tolerance invariants | lab-executed; counters + verdict from `internal/telemetry` + ledger export (Phase 04 Task 4.5) |

## Determinism & replay guards (add early, keep green)

- Replay of any thermal trace performs **zero** external effects
  (`internal/replay` invariant) — assert no serial send occurs under replay.
- The serial effector is inert under the `simulated`/`shadow`/replay profiles;
  only the `emulator` and `physical` profiles reach a transport.
- Golden replay of the thermal spec is byte-stable across runs (mirror the
  existing predictive-maintenance golden-replay test).

## Invariant re-check before each gate

Run against the ten product invariants (design README) — the ones this program
stresses most:

1. Untrusted content is evidence, never instructions — adversarial device
   text/metadata cannot become a target/operation (Phase 02 Task 2.3, Phase 03
   Task 3.2).
2. The model cannot execute effects or name raw device parameters — materializer +
   catalog (Phase 03 Task 3.2).
3. Ambiguity stops — `UnknownOutcomeError` → reconcile, never blind retry (Phase
   03 Task 3.5, Phase 04 Task 4.2).
4. Deterministic replay preserved (guards above).
5. Idempotent effects — `idempotency_key` = one semantic effect, dedup on device
   and in-session.

## Consolidated gate ladder (single source of truth)

| Gate | Phase | Claim licensed |
|---|---|---|
| G2 | 01 | real sensor telemetry ingested with explicit quality/time |
| G3 | 02 | Tamoz shadow decisions compared with a baseline |
| G4a | 03 | one bounded LED effect through the governed action plane |
| G4b | 03 | one independently verified low-voltage mechanical effect |
| G4c | 04 | one governed low-voltage physical loop verified (auth loss, reconciliation, e-stop) — **M2** |
| G5 | 04 | reference rig survives declared faults over an 8-hour soak — **M3** |

Do not advance a gate with a lower-level pass standing in for a higher one, and
never generalize G5 to production, certification, arbitrary devices, or
exactly-once effects.

## Commands

```bash
make ci-check
```

```bash
go test ./internal/actions/... ./internal/eventschema/... ./internal/contractsv1/... ./internal/spec/...
```

```bash
go run ./cmd/agentic-stream validate docs/design/examples/zone-thermal.situation.yaml
```
