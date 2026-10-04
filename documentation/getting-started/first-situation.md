# Author your first SituationSpec

A `SituationSpec` declares how the runtime interprets a domain's evidence. It
defines accepted events, time rules, calculations, and Situation state. It
also defines when reasoning may start, its budget, and the proposals it may
return.

The starter fixture remains under `docs/design/examples` because it is also
used by implementation tests. This page is the maintained authoring guide;
start here for the instructions, then use the example as your working model.

## Start from a known-good example

Build the binary with [the quickstart](quickstart.md), then copy the example
to a temporary working file. This keeps the committed test fixture intact:

```bash
spec_dir=$(mktemp -d)
cp docs/design/examples/predictive-maintenance.situation.yaml "$spec_dir/motor.situation.yaml"
./bin/agentic-stream validate "$spec_dir/motor.situation.yaml"
```

Keep this terminal open so `spec_dir` remains available for the later command.
Edit the copied file with your editor. Start with `metadata.description`, then
validate again. Make one kind of behavioral change at a time; keep a trace and
expected result for each change.

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
./bin/agentic-stream validate "$spec_dir/motor.situation.yaml"
```

The default output reports the spec name, version, schema, and `sha256:` digest.
Use `validate --json` to print the canonical JSON. Equivalent YAML
representations should produce the same canonical identity. Changing the digest
changes the deployed definition's identity.

## Change one part of the story at a time

| You want to change… | Read and edit | Check afterward |
| --- | --- | --- |
| Which evidence is accepted | `inputs` and the event registry | Validate field names, schema, units, and tenant identity |
| How observations are summarized | `time`, `windows`, `operators` | Replay a trace, including duplicates and late evidence |
| When the condition changes phase | `situation` | Check opening, duration, recovery, and version history |
| When reasoning may start | `cognition` | Check eligible and suppressed triggers, queue timing, and budgets |
| Which proposals are allowed | `actions` | Review Intent schemas/risk and current runtime policy limits |

A spec passing validation is the first check. It does not establish that its
rules fit real sensor behavior or that an external effect is safe. Start with
the simulated effect profile and use [the domain guide](../guides/add-a-domain.md)
when adding a schema or action type.

## Domain-data rule

Do not add event schemas, simulator channel mappings, or intent catalogs as Go
literals. Add them to the repository JSON data files and update the deliberate
digest/parity tests. This keeps domain data inspectable, reusable, and aligned
with the cross-language contract.

## Safe authoring checklist

- Use a stable partition key and entity identity.
- Choose a late-data policy deliberately; `correct_and_reconsider` can create
  additional reasoning work to review prior actions.
- Bound every window, episode, tool, model, and cost budget.
- Declare only the Intent types and fields the episode may propose.
- Give each Intent a risk class and a parameter schema.
- Make policy and effector behavior explicit; use simulated effects first.
- Add a deterministic trace and a test for duplicates, lateness, and restart.

## Next reads

- [SituationSpec contract](../contracts/situation-spec.md)
- [Stream-processing design](../design/stream-processing.md)
- [Add a domain](../guides/add-a-domain.md)
