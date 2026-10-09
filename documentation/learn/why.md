# Why Agentic Stream exists

Agentic Stream helps interpret conditions that develop over time. It keeps
track of evidence, asks an agent to reason when useful, and checks any proposed
action before it can execute.

## A reading is a fact; a condition needs context

Imagine a motor that sends vibration and temperature readings. One high
reading could mean wear, a temporary load change, or a faulty sensor. A useful
response depends on what happened before, whether other evidence agrees, and
whether the condition persists.

An alert can tell you that a threshold was crossed. A **Situation** records
the condition as it develops: which motor is involved, its phase, the facts
supporting that phase, and the status of the evidence. A published version
gives the agent a stable starting point.

## Why not ask an agent about every reading?

Readings can arrive faster than a model can analyze them. While a model is
working, new readings may change the condition. A queue of old questions can
consume time and money without improving the response.

The runtime therefore keeps two different jobs separate:

| Job | What it needs | Owner |
| --- | --- | --- |
| Keep track of what happened | Repeatable time rules, identities, windows, and state | Deterministic stream processing |
| Interpret a condition worth attention | Context, uncertainty, a finite objective, and a budget | A bounded agent episode |

**Deterministic** means that the same accepted inputs and configured time rules
produce the same stream result. Model output can vary, so it is evaluated
outside that state-changing stream transaction.

## What the runtime adds

It connects continuous evidence to limited reasoning sessions and governed
actions. It remembers both the condition and the reasons for starting, delaying,
or refusing work. If reasoning proposes an external change, policy checks
current state before that proposal can become a Command.

The model has a useful role: it can interpret evidence and propose a next step.
The runtime keeps the history, limits the work, checks the output, and controls
execution.

## When this shape is useful

Consider this runtime when conditions develop across multiple observations, late
or duplicate evidence matters, and you need to review why a proposed action was
permitted or refused. Predictive maintenance is the first example in this
repository. Other domains need their own schemas, rules, tests, and operational
evidence.

The current implementation starts with a single node and local inputs. It is
not yet qualified for use as a production maintenance service. See the
[product boundaries](../overview/product.md) and
[limitations](../overview/limitations.md).

## Where the reasoning comes from

The design's root-cause analysis
and ADR-002
explain the continuous/episodic separation. The
[stream design](../design/stream-processing.md) shows the implemented path.

## Next reads

- [Follow a reading through the runtime](how-it-works.md)
- [Product overview](../overview/product.md)
