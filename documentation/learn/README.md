# Learn Agentic Stream

Start here if you want to understand the project before reading its contracts
or running it. No knowledge of stream processing or agent frameworks is assumed.

Agentic Stream watches evidence over time, keeps a record of the condition it
reveals, and asks an agent to reason only when that would be useful. A separate
policy and action path checks any proposed change before it can happen.

## The whole idea in four steps

![Four stages: evidence builds a Situation, a useful change may start an episode, and permitted proposals enter governed action.](../assets/runtime-story.svg)

Read left to right: observations build a **Situation**, a durable record of an
evolving condition. A meaningful change may start an **episode**, a limited
reasoning session. The episode proposes an action; policy may permit it, delay
it, or refuse it. The action path records the outcome.

The arrows show the possible path. Many events produce no new Situation;
many versions start no episode; many proposals produce no external effect.
[Diagram source](../assets/runtime-story.mmd) ·
[Architecture evidence](../architecture/overview.md).

## Follow the story

| Step | Main question | What you will learn |
| --- | --- | --- |
| 1. [Why the project exists](why.md) | Why does an event stream need more than alerts? | The problem and the runtime's role |
| 2. [From a reading to a result](how-it-works.md) | What happens to one motor reading? | Evidence, state, reasoning, and governed action |
| 3. [Time and changing state](time-and-state.md) | What if readings arrive late or repeat? | Event time, watermarks, completeness, and versions |
| 4. [When an agent should reason](reasoning.md) | When is a reasoning session worth starting? | Triggers, waiting, budgets, and stale work |
| 5. [From proposal to effect](safe-actions.md) | Who decides what may execute? | Decisions, Intents, policy, Commands, and outcomes |
| 6. [Why these design choices](design-choices.md) | Why build it this way? | Tradeoffs, boundaries, and current limits |

Each page introduces one part of the system. More detailed diagrams appear in
[the design section](../design/README.md), after this basic model.

## Keep the explanation and the proof separate

The motor story illustrates the architecture. The committed opening trace
shows only the stream path; it does not create a maintenance ticket. The
[walkthrough](../guides/predictive-maintenance.md) explains what the fixture and
the separate synthetic tests actually prove.

This is an unreleased development snapshot. Read the
[current status](../overview/status.md) before choosing an integration.

## Next reads

- [Start the story: why the project exists](why.md)
- [Try the local quickstart](../getting-started/quickstart.md)
- [Look up a term](../overview/concepts.md)
