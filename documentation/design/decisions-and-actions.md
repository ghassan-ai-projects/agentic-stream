# Decisions, policy, and actions

A model proposal must pass validation and policy before it becomes a Command.
The dispatcher then checks current readiness before calling an effector. Each
step records what was permitted or refused.

## Two authority boundaries

How is a model proposal accepted?

```mermaid
flowchart LR
    D["Decision and Intents"] --> V["Validate proposal"]
    V --> P["Policy"]
    P -->|permitted| C["Command"]
```

Text equivalent: a proposal must pass binding/schema validation and current
policy before it becomes a Command. Rejection, deferral, or an approval need
can stop this path. Source: [Decision validator](../../internal/decisions/internal/domain/validator.go)
and [policy](../../internal/policy/policy.go).

How is an accepted Command dispatched?

```mermaid
flowchart LR
    C["Leased Command"] --> R["Readiness check"]
    R -->|permitted| E["Effector"]
    E --> O["Outcome"]
```

Text equivalent: dispatch leases an accepted Command, checks current authority,
calls the configured effector only when permitted, and records the result.
Source: [dispatch use case](../../internal/actions/internal/app/dispatch.go).

## Why validate twice?

The first boundary rejects proposals that are malformed, stale, out of scope,
or disallowed. The second accounts for change while an accepted Command waits:
runtime ownership, operator controls, interlocks, or device readiness can change.

The extra checks and durable queue add work and may add latency. They also
keep model output from carrying its own execution authority. A prior approval
cannot become a shortcut past a later stop.

## Validation

Decision validation binds the output to the exact episode attempt, fence,
snapshot digest, Situation version, schema, and allowed catalog. For each
Intent, validation also checks type, risk, parameters, expiry, evidence
references, and its link to a prior Command when proposing compensation.

## Policy

The gateway evaluates against current durable state, not the state observed when
the model started. An Intent stays fresh until the Situation changes
materially: cognition records the latest version for which a trigger's
`materialDelta` holds (or that ends the occurrence), and policy, approval
resolution and dispatch refuse the Intent as `situation_version_stale` only
when such a version is newer than the Intent's. A window's provisional and
on-time versions alone do not cancel a pending decision. A trigger without
`materialDelta` keeps the strict rule: any newer version is stale
(ADR-018 (`docs/design/DECISIONS.md`)). Approval-required paths create durable approval records;
automatic paths still pass all revalidation and interlock checks. Drain and kill
controls apply to the current policy epoch, the generation of operational
permission. A worker response cannot bypass an operator stop.

## Action and outcomes

Commands are persisted in an outbox with a stable idempotency key. The
dispatcher leases a command, revalidates it, and invokes the concrete effector.
The simulated effector is used by the proof suite; the watch effector is a
bounded CEL-based mechanism with owner/interlock fencing.

If the provider may have accepted a request but the runtime cannot prove the
result, the dispatcher records an unknown outcome and waits for reconciliation.
It does not blindly retry an effect that could duplicate external work.

## Source evidence

- Decision validation: [`internal/decisions/internal/domain/validator.go`](../../internal/decisions/internal/domain/validator.go)
- Policy gateway: [`internal/policy/policy.go`](../../internal/policy/policy.go)
- Dispatcher: [`internal/actions/internal/app/dispatch.go`](../../internal/actions/internal/app/dispatch.go)
- Effect adapters: [`internal/device/`](../../internal/device/) and [`internal/watch/`](../../internal/watch/)
- Approved effect contracts: [`internal/actionport/`](../../internal/actionport/)

## Next reads

- [Decision and Intent contract](../contracts/decision-intent.md)
- [Security model](../architecture/security-model.md)
- [Durability](../architecture/durability.md)
