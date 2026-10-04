# From a reading to a result

For new readers: follow a motor observation through the system. The first
step uses the committed fixture; later steps describe the possible runtime
path rather than claiming that this one reading executes an action.

## 1. Accept evidence

The opening trace contains a vibration reading for `motor-17`: `5.0 mm/s`.
The event includes an identity, tenant, source, and time. Ingress, the input
boundary, checks the envelope and payload schema before the event log accepts it.

An exact duplicate is recognized by stable identity and matching payload digest.
A conflicting reuse of that identity is rejected. A reading remains evidence;
it cannot ask the runtime to execute a command.

Source: [opening trace](../../examples/predictive-maintenance/testdata/trace-opening.jsonl)
and [event contract](../contracts/event-envelope.md).

## 2. Update the condition

The stream engine applies the SituationSpec: the domain's declared inputs,
time rules, calculations, lifecycle, and triggers. A **window** selects the
readings used in a calculation. An **operator** calculates a feature such as
vibration over that window. A **reducer** uses features to update Situation state.

The committed motor fixture opens a bearing-degradation Situation when its
opening rule is met. Its initial phase is `candidate`. Later phase transitions
require supporting conditions to last for their declared durations.

A **version** records the published state at that point. Opening, phase changes,
and completeness changes can publish another version; a fact update alone
need not publish. Old versions remain fixed. [The domain model](domain-model.md)
explains the difference between current state and a published snapshot.

Source: [motor SituationSpec](../../docs/design/examples/predictive-maintenance.situation.yaml)
and [time and state](time-and-state.md).

## 3. Decide whether reasoning is useful

The motor fixture's diagnosis trigger concerns `warning` or `incident` phases
and also checks heartbeat evidence, score, and material change. The single
opening reading remains in `candidate`, so it starts no episode.

If a later version passes its trigger and queue timing/capacity gates, admission
can create an episode. That episode receives an immutable snapshot, an
objective, allowed tools and Intent types, a deadline, and a budget.

Source: [reasoning and admission](reasoning.md).

## 4. Evaluate the proposal

The executor can return a typed **Decision** containing an **Intent**, such as
proposing a maintenance ticket. The runtime validates its identity, snapshot
binding, schema, and allowed vocabulary. Policy then checks the proposal
against current state and permissions.

An accepted Intent can become a durable **Command**. The dispatcher checks
readiness again before calling the configured effector, the adapter that
performs the change. It records success, failure, or an uncertain outcome.

Source: [the governed action path](safe-actions.md).

## What you will see when you run the opening trace

| Counter | Expected | Meaning |
| --- | --- | --- |
| Events ingested/processed | One each | The stream accepted and processed the reading |
| Episodes admitted/executed | Zero | The candidate phase did not meet the diagnosis trigger |
| Intents/Commands | Zero | No episode proposed work; no effect was dispatched |

Zero actions is the expected result. The separate
[synthetic end-to-end test](../../internal/runtime/pipeline_e2e_test.go)
`TestPipelineCompletesDecisionToSimulatedOutcome` proves the later path with a
small test-specific spec, fake executor, and simulated effector. It does not
certify a physical motor or ticket provider.

## Next reads

- [What a Situation represents](domain-model.md)
- [Time and changing state](time-and-state.md)
- [Run the predictive-maintenance walkthrough](../guides/predictive-maintenance.md)
- [SituationSpec authoring](../getting-started/first-situation.md)
