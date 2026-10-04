# Cognition and bounded episodes

Cognition is a deterministic admission decision. The scheduler decides when a
Situation deserves a bounded agent episode; the agent does not decide whether
to wake up for every incoming event.

## Admission path

Why does one version start work while another does not?

```mermaid
flowchart TD
    V["Situation version"] --> T["Evaluate trigger and score"]
    T -->|ineligible| R["Record reason; no episode"]
    T -->|eligible| Q["Queue with timing and capacity gates"]
    Q -->|due and admissible| E["Create bounded episode"]
```

Text equivalent: an ineligible version records a reason without creating an
episode. An eligible version can enter a timed queue; only due, admissible work
becomes an episode. This is a conceptual summary, not the exact ordering of
every validation check. Source: [cognition](../../internal/cognition/) and
[admission](../../internal/admission/).

## Why a scheduler instead of a model call per event?

The scheduler spends reasoning effort on declared opportunities. Timing and
supersession prevent a long queue of outdated questions from becoming the
model's workload. Recorded trigger and admission reasons also explain why the
runtime stayed quiet.

This adds domain choices: a trigger or budget that is too restrictive can miss
useful attention; a loose one can spend budget on repeated conditions. Use
replay and the durable evaluations to review those choices before treating them
as deployment-qualified behavior.

## Controls

- **Trigger condition:** deterministic restricted CEL.
- **Score/threshold:** avoids waking an executor for weak evidence.
- **Material delta:** allows a trigger to require a meaningful change.
- **Debounce:** sets the earliest run time after a queue opportunity is created.
- **Cooldown:** delays work until the configured interval after the latest trigger admission.
- **Coalescing:** merges pending work for the same logical condition.
- **Capacity:** defers when bounded concurrency or budget capacity is used.
- **Freshness/completeness:** prevents reasoning from stale or uncertain state.
- **Supersession:** cancels work whose snapshot no longer represents the live
  Situation.

## Episode contract

An episode binds one immutable snapshot, trigger, tenant/entity, executor and
prompt/objective digests, allowed evidence/tools, allowed Intent types, risk
ceiling, dispatch policy, deadline, and finite budget. The model can propose a
Decision; the episode runtime and policy plane validate everything afterward.

Budgets cover wall time, model calls, input/output tokens, tool calls,
tool-result bytes, provider retries, and cost where configured. Budget updates
are durable and a late provider response cannot extend the deadline.

## Cancellation and reconsideration

Material supersession cancels an active attempt. The worker's attempt ID and
fence ensure a late result cannot be accepted. A late correction can create one
deduplicated reconsideration episode bound to the corrected Situation version.

## Source evidence

- Scheduler: [`internal/cognition/`](../../internal/cognition/)
- Episode lifecycle: [`internal/episodeledger/lifecycle.go`](../../internal/episodeledger/lifecycle.go)
- Episode assembler: [`internal/episodes/assembler.go`](../../internal/episodes/assembler.go)
- Cancellation/recovery tests: [`internal/episodes/`](../../internal/episodes/)

## Next reads

- [Core concepts](../overview/concepts.md)
- [Worker boundary](../architecture/worker-boundary.md)
- [Decisions and actions](decisions-and-actions.md)
