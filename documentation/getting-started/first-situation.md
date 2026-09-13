# Author your first SituationSpec

Situation behavior is data-driven. A `SituationSpec` declares the input event
schemas, event-time policy, windows, deterministic operators, Situation state,
cognition triggers, executor budget, and allowed Intent catalog.

The starter fixture remains under `docs/design/examples` because it is also
used by implementation tests. This page is the maintained authoring guide;
the archive path is not a second public documentation entrypoint.

## Start from a known-good example

Copy the predictive-maintenance example conceptually, then change one domain
area at a time:

```text
docs/design/examples/predictive-maintenance.situation.yaml
```

The schema is committed at
[`internal/spec/schema.json`](../../internal/spec/schema.json). The compiler
also performs semantic checks that a JSON Schema alone cannot express.

## Required top-level sections

Every v1 spec contains:

| Section | Role |
| --- | --- |
| `apiVersion`, `kind` | Bind the authoring contract |
| `metadata` | Name, semantic version, description, labels |
| `inputs` | Event type, schema, partition key, entity, classification |
| `time` | Watermark, disorder, idle, lateness, late-data policy |
| `windows` | Tumbling, sliding, count, or decay boundaries |
| `operators` | Deterministic feature calculations |
| `situation` | Lifecycle, phase, severity, reducers, completeness |
| `cognition` | Trigger conditions, scores, lanes, budgets, executor |
| `actions` | Intent types, risk classes, parameter schemas, policy |

Runtime telemetry and data retention are deployment concerns in v1 and are not
authoring fields in the SituationSpec. Do not add `retention` or `telemetry`
sections to a spec until an enforcing contract is introduced.

## Validate before deploying

```bash
./bin/agentic-stream validate path/to/example.situation.yaml
```

The compiler emits canonical JSON and a `sha256:` digest. Equivalent YAML
representations should produce the same canonical identity. A digest change is
an explicit deployment/versioning event.

## Domain-data rule

Do not add event schemas, simulator channel mappings, or intent catalogs as Go
literals. Add them to the repository JSON data files and update the deliberate
digest/parity tests. This keeps domain data inspectable, reusable, and aligned
with the cross-language contract.

## Safe authoring checklist

- Use a stable partition key and entity identity.
- Choose a late-data policy deliberately; `correct_and_reconsider` has a cost
  and an action-governance consequence.
- Bound every window, episode, tool, model, and cost budget.
- Declare only the Intent types and fields the episode may propose.
- Give each Intent a risk class and a parameter schema.
- Make policy and effector behavior explicit; use simulated effects first.
- Add a deterministic trace and a test for duplicates, lateness, and restart.

## Next reads

- [SituationSpec contract](../contracts/situation-spec.md)
- [Stream-processing design](../design/stream-processing.md)
- [Add a domain](../guides/add-a-domain.md)
