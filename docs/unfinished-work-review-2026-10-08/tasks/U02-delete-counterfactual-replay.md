# U02 — Delete counterfactual replay mode

Status: done · Decision: **delete from code and plans** · Priority: P2 · Size: S

## Finding

`replay.ModeCounterfactual` sends `Capabilities.Commands` to a
`Capabilities.Simulator` (`internal/replay/internal/app/counterfactual.go`).
Four functions are reachable only from tests: `applyCounterfactual`,
`simulateCommands`, `simulateCommand`, `AdmitSimulatedCommand`.

- There is no production `Simulator` implementation; the only ones are test
  doubles (`countingSimulator`, `testSimulator`).
- The commands are supplied by the caller, not derived from the replayed
  Decisions, so the mode does not answer "what would have happened".
- Nothing outside `internal/replay` mentions counterfactual except the unused
  `replay_jobs.mode` check constraint.

## Decision and reasoning

Delete it. Invariant 9 *allows* an explicit simulation mode; no acceptance item
requires one. Gate C only says counterfactual mode "uses an explicit simulator
only", a constraint on something that would exist. The real need, rehearsing
effects without hardware, is already served by the live device `emulator`
profile (Streams Simulator over the gateway link), which exercises the real
policy and action plane instead of a parallel path.

Keeping it means a third worker-aware mode to wire into the CLI (U20/U21) with
no simulator and no command source to feed it.

## Steps

1. Delete `internal/replay/internal/app/counterfactual.go`,
   `internal/replay/internal/domain/counterfactual.go`, the `Simulator`,
   `SimulatedCommand`, `Capabilities.Simulator/Commands` fields, the
   `ModeCounterfactual` constant and its facade aliases, and their tests.
2. Remove `Result.SimulatedResults` if nothing else sets it.
3. Keep `ErrUnsupportedMode`: `"counterfactual"` now returns it, and a test pins
   that.

## Done when

- `grep -ri counterfactual internal cmd` returns nothing outside the historical
  migration `001_initial.sql`.
- Plan and doc rows in [PLAN_CHANGES.md](../PLAN_CHANGES.md) P04 are applied
  (IMPLEMENTATION_PLAN M3.5, BUILD_COMPLETION_BAR Gate C wording,
  `documentation/design/replay-and-shadow.md`, `documentation/overview/concepts.md`,
  `documentation/learn/safe-actions.md`, `documentation/architecture/durability.md`,
  `docs/eval` A9/S5.2).

## Result

Mode, simulator types, admission rule and their tests are gone; `"counterfactual"`
returns `ErrUnsupportedMode` (`TestCounterfactualModeIsRefused`). The worker
protocol's `counterfactual_capable` handshake field stays: the proto is a wire
contract vendored by Tamoz and pinned. The `replay_jobs` check constraint goes
with the table in U11. Dated module records (`docs/replay-reference-module-*`)
are history and stay unchanged.
