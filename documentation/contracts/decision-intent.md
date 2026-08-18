# Decision and Intent contract

The Decision contract is the model-facing output boundary. It is a proposal,
not an authorization. The runtime validates it, then policy evaluates each
Intent independently.

## Shared v1 schemas

The runtime validates these embedded schemas under
[`internal/contractsv1/schemas/v1/`](../../internal/contractsv1/schemas/v1/):

- snapshot;
- decision;
- intent;
- command;
- outcome;
- trigger evaluation.

The `decision-v1.json` contract binds the Decision to `decision_id`,
`episode_id`, `attempt_id`, `fence`, and `snapshot_digest`, and bounds confidence
and Intent count. The `intent-v1.json` contract binds each Intent to its
Decision, tenant, Situation version, risk class, parameters, and expiry.

## Validation order

The runtime checks:

1. JSON shape and required fields;
2. canonical digest and domain separation;
3. episode/attempt/fence/snapshot/Situation identity;
4. allowed Intent type and catalog binding;
5. parameter schema and evidence references;
6. expiration, budget, freshness, completeness, and lifecycle state;
7. policy, approval, quota, rate limit, principal, interlock, calibration, and
   current epoch before command creation/dispatch.

An invalid, stale, or superseded result is rejected and recorded with a reason.

## Risk classes

Intent risk is explicit (`R0` through `R4`). The exact policy and approval
requirements are spec- and policy-versioned. Do not infer that a low-risk
Intent is automatically safe for every deployment.

## The safety boundary

```text
model output -> Decision validator -> Intent validator -> policy gateway
             -> approval/interlock/epoch checks -> command outbox -> effector
```

No model output bypasses the validator or creates a Command directly.

## Source evidence

- Schemas: [`internal/contractsv1/schemas/v1/`](../../internal/contractsv1/schemas/v1/)
- Validation: [`internal/decisions/validator.go`](../../internal/decisions/validator.go)
- Policy: [`internal/policy/policy.go`](../../internal/policy/policy.go)
- Action dispatch: [`internal/actions/dispatcher.go`](../../internal/actions/dispatcher.go)

## Next reads

- [Security model](../architecture/security-model.md)
- [Decisions and actions design](../design/decisions-and-actions.md)
- [Notification contract](notifications.md)
