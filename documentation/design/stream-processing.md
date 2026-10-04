# Stream processing

The stream plane turns unbounded evidence into deterministic, versioned
Situations without invoking a model for every event.

## Event-time path

What happens inside the stream path?

```mermaid
flowchart TD
    E["Validate and deduplicate evidence"] --> T["Apply event-time rules"]
    T --> F["Calculate windowed features"]
    F --> S["Reduce Situation state"]
    S --> V["Publish a version when needed"]
```

Text equivalent: the stream validates evidence, applies time rules, calculates
features, and reduces state. A change may publish a version. This diagram omits
transaction bookkeeping; it does not add model calls or effects to the path.
Source: [engine](../../internal/engine/) and [event log](../../internal/eventlog/).

The engine serializes state changes per virtual partition. A later event can
produce a correction version, but it cannot rewrite a published version.

## Why windows and explicit time rules?

A window gives a calculation a declared evidence horizon. The motor spec uses
a fifteen-minute vibration window rather than treating one reading as the
whole condition. The input time and late-data policy remain part of the result,
so a transport delay does not silently become a different domain history.

The tradeoff is configuration: developers must choose a time horizon, disorder
allowance, idle behavior, and late policy suitable for their sources. Waiting
for more evidence can improve completeness while delaying a result. The
[time and state introduction](../learn/time-and-state.md) explains this without
requiring the full timing contract.

## Time and lateness

`SituationSpec.time` declares bounded out-of-orderness, source idle timeout,
allowed lateness, clock-skew tolerance, and one of these late policies:

| Policy | Meaning |
| --- | --- |
| `drop_with_audit` | Do not change state; retain the decision as an audit |
| `history_only` | Preserve evidence without changing the derived Situation |
| `correct` | Recompute the affected state and publish a correction |
| `correct_and_reconsider` | Correct state and reevaluate for a deduplicated reconsideration, subject to eligibility |

Missing heartbeat and source-health signals can make completeness uncertain.
Incomplete evidence is explicit state, not a silent default.

## Operators and windows

The current deterministic operators include aggregates, slope-like features,
and missing-heartbeat handling. Window kinds include tumbling, sliding, count,
and decay. The compiler checks that operator inputs, windows, and output names
are declared and compatible.

## Situation versions

A version carries the derived facts, lifecycle/phase, severity, completeness,
material delta, provenance summary, source references, and canonical digest.
The cognitive scheduler consumes the immutable snapshot and its version—not a
mutable pointer that can change underneath an episode.

## Source evidence

- Ingress: [`internal/ingress/`](../../internal/ingress/)
- Event log: [`internal/eventlog/`](../../internal/eventlog/)
- Engine: [`internal/engine/`](../../internal/engine/)
- Operators: [`internal/operators/`](../../internal/operators/)
- Situations: [`internal/situations/`](../../internal/situations/)

## Next reads

- [Event contract](../contracts/event-envelope.md)
- [Cognition](cognition.md)
- [Durability](../architecture/durability.md)
