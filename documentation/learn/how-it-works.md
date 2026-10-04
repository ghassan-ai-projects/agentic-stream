# From a reading to a result

Follow one motor reading through the system. The first step uses the
repository's test data. The later steps explain what could happen as the
condition develops; this reading alone does not cause an external action.

## 1. Accept evidence

The opening trace contains a vibration reading for `motor-17`: `5.0 mm/s`. The
event includes an identity, tenant, source, and time. **Ingress**, the input
handler, checks the event's identifying fields and payload against their schemas
before the event log accepts it.

The runtime recognizes a duplicate when its event identity and payload digest
match the stored event. It rejects reuse of that identity with a different
payload. A reading remains evidence;
it cannot ask the runtime to execute a command.

Source: [opening trace](../../examples/predictive-maintenance/testdata/trace-opening.jsonl)
and [event contract](../contracts/event-envelope.md).

## 2. Update the condition

The stream engine applies the SituationSpec: the domain's declared inputs,
time rules, calculations, lifecycle, and triggers. A **window** selects the
readings used in a calculation. An **operator** calculates a feature such as
vibration over that window. A **reducer** uses features to update Situation state.

The motor example opens a bearing-degradation Situation when its
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
proposing a maintenance ticket. The runtime checks which episode and snapshot it belongs to, whether its
structure is valid, and whether the Intent type is allowed. Policy then checks the proposal
against current state and permissions.

An accepted Intent can become a durable **Command**. The dispatcher checks
current permission and readiness again before calling the configured
**effector**, the adapter that performs the change. It records success, failure, or an uncertain outcome.

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
