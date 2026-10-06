# Spec ubiquitous language

| Term | Meaning | Code name | Storage name |
| --- | --- | --- | --- |
| SituationSpec | The authored YAML document: inputs, time policy, windows, operators, situation, cognition and actions. | `Compiler.CompileFile`, `CompileFile` | spec file |
| Compiled spec | The immutable, validated result with its canonical JSON and digest. Compilation reports every problem with its path. | `CompiledSpec`, `CompileError` | `compiled_ir` |
| Spec digest | The digest of the canonical JSON; it identifies a deployment and is part of replay evidence. | `CompiledSpec.Digest` | `spec_sha256`, `deployment_id` |
| Input | An accepted event schema and how it maps to a partition key, entity type and classification. | `Input` | `inputs` |
| Time policy | Event-time semantics: watermark strategy, out-of-orderness, idle timeout, allowed lateness, late policy. | `TimePolicy` | `time` |
| Window | A named window: kind, size and slide. | `Window` | `windows` |
| Operator | A declared stream computation (see `operators`). | `Operator` | `operators` |
| Situation (spec) | The declared situation: type, entity key, occurrence rules, phases, transitions, reducers. | `Situation`, `Occurrence`, `Phase`, `Transition`, `Reducer` | `situation` |
| Trigger | A condition on a new Situation version, with a score, that makes reasoning worth considering. | `Trigger` | `cognition.triggers[]` |
| Delta | The comparison of a new Situation version to the last reasoned version, exposed to trigger expressions. | `DeltaKeys` | CEL variable `delta` |
| Executor (spec) | The named executor, objective, prompt and budget for an episode. | `Executor`, `Budget`, `SkillRef` | `cognition.executor` |
| Intent catalog | The intent types an episode may propose, each with a risk class and parameter schema. | `Intent`, `Actions` | `actions.intents[]` |
| Deployment | A compiled spec stored as the active version of its name. A new version retires the previous one; redeploying the same digest is idempotent. | `SaveDeployment` | `spec_deployments` |
| Duration string | The runtime's textual duration: any `time.ParseDuration` unit plus whole days (a fixed 24 hours, positive, no overflow). All spec duration fields use it. | `ParseDuration` | `15m`, `6h`, `30d` |
| Event schema | A schema bound to one normalized event type and version, with its payload fields keyed by name. Domain data, never a Go literal. | `EventSchema`, `EventField` | `event_schema_data.json` |
| Schema reference | The name a spec input uses to select an event schema. | `LookupEventSchema` | `schemaRef` in the spec |
| Registered schema | One immutable, digested event schema version stored for the event log to validate against. Re-registering identical bytes is allowed; different bytes are refused. `spec` is the only writer of the table. | `RegisterEventSchema` | `event_schemas` |

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| Config, manifest | Spec | A SituationSpec is compiled and digested; configuration is not. |
| Rule | Trigger, transition, or reducer | Each has its own semantics. |
