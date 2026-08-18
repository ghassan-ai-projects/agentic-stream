# Architecture decision records

The current decision record is the accepted design document
[`docs/design/DECISIONS.md`](../../docs/design/DECISIONS.md). The public pages
summarize its durable choices; they do not invent a second decision history.

## Accepted decision themes

| Theme | Current choice | Public explanation |
| --- | --- | --- |
| Runtime shape | Modular monolith first | [Deployment model](../design/deployment-model.md) |
| Persistence | SQLite WAL | [Durability](../architecture/durability.md) |
| Authoring | YAML/JSON SituationSpec + canonical digest | [SituationSpec](../contracts/situation-spec.md) |
| Rules | Restricted deterministic CEL | [Stream processing](../design/stream-processing.md) |
| Cognition | Deterministic scheduler before bounded episode | [Cognition](../design/cognition.md) |
| Model boundary | Typed proposals, no effector access | [Security model](../architecture/security-model.md) |
| Effects | Policy/action separation, outbox, idempotency | [Decisions and actions](../design/decisions-and-actions.md) |
| Worker boundary | Current-v1 Go Protobuf/gRPC over private socket | [Worker protocol](../contracts/worker-protocol.md) |
| Replay | Effect-disabled by construction | [Replay and shadow](../design/replay-and-shadow.md) |
| Scale-out | Deferred until single-node semantics are proven | [Product overview](../overview/product.md) |

## When to add a decision

Write or update the authoritative decision record when a change affects
invariants, public contracts, storage meaning, worker compatibility, security
authority, deployment shape, or deferred scope. Include context, decision,
alternatives, consequences, and evidence.

## Historical decisions

The v0 and v0.1 design directories are historical iterations. They are useful
for rationale and critique, but are not current implementation authority. See
the [archive guide](../../docs/README.md).

## Next reads

- [Design](../design/README.md)
- [Quality](../governance/quality.md)
- [Contributing](../../CONTRIBUTING.md)
