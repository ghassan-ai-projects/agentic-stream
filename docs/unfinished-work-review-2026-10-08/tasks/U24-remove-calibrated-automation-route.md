# U24 — Remove the calibrated-automation route

Status: todo · Decision: **remove from code and plan** · Priority: P2 · Size: S

## Finding

For R2 intents, policy takes the `calibration` route
(`internal/policy/internal/domain/routing.go`): if
`calibration_artifacts` holds an active artifact for the Situation type and
executor version, the intent dispatches automatically with reason
`calibrated_automation`; otherwise it needs approval.

When `qualification` was dissolved, its only writer (`CalibrationStore.Activate`)
was deleted because nothing called it. `calibration_artifacts` now has a reader
(`CalibrationActive`) and no writer, so the branch can never be taken. The
evaluation work that would produce calibration evidence (`docs/eval/`) is
"design only, not implemented". The HIL plan's "calibration" means sensor
calibration, not model calibration.

## Decision and reasoning

Remove the route: R2 always requires approval. That is production behavior
today, so nothing changes at runtime.

- Adding a provisioning command instead would let an operator declare a model
  "calibrated" with no evidence pipeline behind it. That is weaker than human
  approval, on the path that decides whether a consequential effect runs
  without a human.
- A branch that can never be taken on the safety path is exactly the false
  coverage this review is removing.
- When the evaluation suite exists and produces signed calibration evidence,
  automated R2 should be designed with it (who signs, what expires, how it is
  revoked), not resurrected from this table.

## Steps

1. `RiskRoute` returns `approval` for R2; delete `routeConsequentialIntent`,
   `CalibrationActive` and its store query.
2. New migration dropping `calibration_artifacts` (no foreign keys reference it).
3. Keep a test pinning that R2 needs approval and never reports
   `calibrated_automation`.

## Done when

- `grep -ri calibration internal/policy` is empty.
- `documentation/overview/status.md`, `documentation/contracts/decision-intent.md`
  and `documentation/learn/runtime-boundaries.md` no longer list calibration as a
  policy input; `docs/eval` notes that automated R2 needs a new design.
