# Architecture overview

Agentic Stream is one application with separate internal modules. Stream
processing follows repeatable rules, reasoning has explicit limits, and every
external effect must pass through policy and action dispatch.

## Main runtime boundaries

For a first explanation, read [the four-stage story](../learn/README.md).
This diagram adds the runtime owners. Detailed calculations and recovery
steps appear on their own design pages.

What are the major handoffs?

```mermaid
flowchart TD
    E["Evidence log"] --> S["Situation versions"]
    S -->|eligible change| C["Scheduler and admission"]
    C --> B["Bounded episode"]
    B -->|typed proposal| P["Validation and policy"]
    P -->|permitted Command| A["Dispatch and outcome"]
```

Text equivalent: accepted evidence becomes versioned stream state. An eligible
change can reach admission and bounded reasoning. Its typed proposal passes
validation and policy before governed dispatch. Each arrow is a possible
handoff, not a promise that every event traverses the whole path.
Source: [runtime composition](../../internal/runtime/pipeline.go).

API notifications observe durable records. They do not add an alternate path
to execution. [Observability](../design/observability.md) explains that separate
relationship.

## Planes and authority

| Plane | Owns | Does not own |
| --- | --- | --- |
| Ingress/event log | Normalize, validate, identify, deduplicate, quarantine | Situation meaning or actions |
| Stream | Event-time state, windows, operators, Situation versions | Model calls or effectors |
| Cognition | Decide whether a version deserves a bounded episode | Policy approval or effects |
| Episode/executor | Read a snapshot, use scoped evidence, propose output | Mutating stream state or executing commands |
| Policy | Revalidate state, identity, risk, approval, interlock, epoch | Model reasoning or provider calls |
| Action | Lease, dispatch, idempotency, reconcile, record outcome | Changing a Decision into a new intent |
| API/notifications | Health, metrics, control, cursor-resumable observation | Bypassing durable ownership and policy |

## Dependency direction

Data flow and package imports are different maps. The diagram above shows
work moving through the runtime. The [module ownership map](modules.md) shows
which packages may depend on which owners, including lifecycle ledgers,
runtime control, and device ports.

The runtime setup code connects the implementations. Input and protocol
handlers stay at the edge; each module uses only the dependencies permitted
by the ownership rules. Shared SQLite
storage does not grant every module permission to change every lifecycle.
Layer and SQL ownership tests enforce these rules.

The runtime keeps model calls and effector calls outside the deterministic
stream transaction. This preserves ordering and makes the stream history
repeatable without waiting on external services.

## Deployment shape

The default deployment is one Go process with SQLite WAL. A separate Go
EpisodeWorker may run over a private Unix socket; mTLS can be configured for a
worker connection. The Go process hosts the evidence reverse service when that
feature is enabled.

The current design does not distribute state across nodes. Ownership leases, virtual
partitions, durable checkpoints, and idempotent ledgers establish the single-node
behavior before any broker or distributed scheduler is introduced.

## Source evidence

- Runtime composition: [`internal/runtime/pipeline.go`](../../internal/runtime/pipeline.go)
- CLI wiring: [`cmd/agentic-stream/main.go`](../../cmd/agentic-stream/main.go)
- Repository map: [repository-map.md](repository-map.md)

## Next reads

- [Understand the design choices](../learn/design-choices.md)

- [Business modules and ownership](modules.md)
- [Durability and recovery](durability.md)
- [Worker boundary](worker-boundary.md)
- [Security model](security-model.md)
- [Public design](../design/README.md)
