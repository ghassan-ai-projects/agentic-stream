# Decisions, policy, and actions

The action path is deliberately longer than “model output → API call.” Each
boundary turns an untrusted proposal into a more constrained durable record.

## Governance flow

```mermaid
sequenceDiagram
    participant M as Model / worker
    participant E as Episode runtime
    participant V as Decision validator
    participant P as Policy gateway
    participant O as Outbox
    participant D as Dispatcher
    participant F as Effector
    M-->>E: Decision + typed Intents
    E->>V: Schema, identity, digest, fence, budget
    V-->>E: accepted or durable rejection
    E->>P: Evaluate each accepted Intent
    P->>P: Freshness; completeness; risk; approval
    P->>P: Quota; rate limit; interlock; calibration; epoch
    P->>O: Governed Command + idempotency key
    O->>D: Leased command
    D->>D: Revalidate immediately before effect
    D->>F: Authorized dispatch
    F-->>D: success, failure, or unknown
    D->>O: Durable outcome / reconciliation state
```

Text equivalent: a worker proposal is validated, policy rechecks current state,
the outbox records an idempotent command, the dispatcher revalidates before an
effector call, and the result is recorded as success, failure, or unknown.

## Validation

Decision validation binds the output to the exact episode attempt, fence,
snapshot digest, Situation version, schema, and allowed catalog. Intent
validation also checks type, risk, parameters, expiry, evidence references,
and compensating binding where applicable.

## Policy

The gateway evaluates against current durable state, not the state observed
when the model started. Approval-required paths create durable approval records;
automatic paths still pass all revalidation and interlock checks. Epoch drain
and kill controls operate at the governance boundary so a worker cannot outrun
an operator stop.

## Action and outcomes

Commands are persisted in an outbox with a stable idempotency key. The
dispatcher leases a command, revalidates it, and invokes the concrete effector.
The simulated effector is used by the proof suite; the watch effector is a
bounded CEL-based mechanism with owner/interlock fencing.

If the provider may have accepted a request but the runtime cannot prove the
result, the dispatcher records an unknown outcome and waits for reconciliation.
It does not blindly retry an effect that could duplicate external work.

## Source evidence

- Decision validation: [`internal/decisions/validator.go`](../../internal/decisions/validator.go)
- Policy gateway: [`internal/policy/policy.go`](../../internal/policy/policy.go)
- Dispatcher: [`internal/actions/dispatcher.go`](../../internal/actions/dispatcher.go)
- Effectors: [`internal/actions/`](../../internal/actions/)

## Next reads

- [Decision and Intent contract](../contracts/decision-intent.md)
- [Security model](../architecture/security-model.md)
- [Durability](../architecture/durability.md)
