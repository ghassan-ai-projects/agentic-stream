# X03 — Material freshness for intents, approvals and dispatch

Status: todo (needs ADR-018 accepted) · Decision: **complete** · Priority: P0 (experiment core) · Size: M · Depends on: X01

Design and reasoning: [EXPERIMENT_DESIGN.md](../EXPERIMENT_DESIGN.md) G2, D1.

## Finding

Three gates require `situations.current_version == intent.situation_version`:

- `policy/internal/domain/routing.go:33` (`FreshnessFailure`, at evaluation),
- `policy/internal/domain/routing.go:62` (`ApprovalDisposition`, at approval),
- `actions/internal/domain/authorization.go:107` (`RequireCurrent`, at dispatch).

Live telemetry publishes a version on every fact change, so any intent older
than a few seconds is `stale`. The joined run passed only because its feed
stopped.

## Steps

1. Write ADR-018 "Material freshness" in `docs/design/DECISIONS.md` from D1, and
   amend TECHNICAL_DESIGN §11.5 and §13.1. Owner accepts it before code.
2. Failing test first (X01 scenario): continuous feed of non-material readings
   while an R1 intent goes through policy and dispatch → today `stale`; must
   dispatch. Second test: a material change (phase → `recovering`) between
   decision and dispatch → must be `superseded`, no command.
3. Cognition: when `supersedePending` runs for an admitted version, mark the
   open intents of older versions of the same Situation and trigger
   `superseded`. Write the status through a policy-owned port, the same way it
   withdraws approvals through `approvalledger` (cognition must not write policy
   tables directly; ADR-017).
4. Policy and actions: replace the equality checks with "not superseded,
   withdrawn or expired; occurrence open". Keep the reason codes distinct
   (`situation_superseded` vs `situation_closed`), so explanations stay exact.
5. Replay: golden hashes must not change. Deterministic replay does not
   dispatch, and supersession is already deterministic.

## Done when

- Both tests pass; X01 gains the continuous-feed scenario and passes.
- An approval resolved after 30 s of non-material readings dispatches; one
  resolved after a phase change is refused as superseded.
- `documentation/design/decisions-and-actions.md` explains freshness in one
  paragraph.
