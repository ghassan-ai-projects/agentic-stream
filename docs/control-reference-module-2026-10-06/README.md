# Control as a reference module — October 2026

Working record for merging `internal/costcontrol` into `internal/control` and rebuilding
the result to the reference-module standard, following the
[reference module refactor prompt](../../.agents/prompts/reference-module-refactor.md).
Backward compatibility is not a goal.

- [Findings](FINDINGS.md)
- [Ubiquitous language](../../internal/control/UBIQUITOUS_LANGUAGE.md)
- [Target design](DESIGN.md)
- [Plan](PLAN.md)

## Summary

`control` is the runtime's control plane: the singleton owner lease that fences writers, the
epoch drain/kill record, and the final dispatch readiness gate. `costcontrol` is the other half
of the same plane: cost ceilings, the kill switch, and per-episode reservations and
settlements. Both are stateless operations over the caller's transaction plus a few
database-level calls, both are consulted by admission, episodes, runtime and actions, and
`control.Kill` already settles cost reservations. They are one module that happened to be two
packages.

The merge was blocked by one edge: `episodeledger` imported `costcontrol` while `control`
imports `episodeledger` (a cycle once merged). The edge was a function-shaped dependency
(`Settle`), so `episodeledger` now takes a small `CostSettler` port that `control`'s cost
ledger satisfies without `episodeledger` importing it.

```
internal/control/                  facade: RuntimeOwner, EpochControl, CostLedger, dispatch authorization
internal/control/internal/app/     use cases: claim/renew/release/assert, drain/kill/assert, reserve/settle/limits
internal/control/internal/domain/  rules: epoch states and refusals, lease validity, cost admission and
                                   settlement decisions, ceiling merge
internal/control/internal/store/   the only SQL for runtime_owner, epoch_control, cost_limits, cost_reservations
```
