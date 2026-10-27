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
| Event-schema registry | The immutable set of event schemas a spec compiles against: the built-in ones, plus those loaded from a directory (`--event-schemas`). A directory may add schemas, never change one; each loaded schema is checked before it is used. | `EventSchemas`, `BuiltinEventSchemas`, `LoadEventSchemas`, `WithEventSchemas` | `event_schemas_digest` (logged at start) |
| Schema reference | The name a spec input uses to select an event schema; resolved in the registry the spec was compiled against. | `CompiledSpec.EventSchema`, `EventSchemas.Lookup` | `schema` of a spec input |
| Dispatched intent | An intent type an active-dispatch spec may turn into a command; a shadow spec dispatches none. Every one needs an effector that owns its route before the runtime starts. | `CompiledSpec.DispatchedIntentTypes` | `actions.intents[].type` |
| Baseline choice | The value the deterministic replay baseline writes for a model-writable enum parameter in one Situation phase. It must be an enum value the parameter's schema accepts. | `IntentBaseline` | `actions.intents[].baseline.byPhase` |
| Promotion block | What rule promotion reads from a spec: the settle window of each diagnosis code, the context features an envelope may cover, the optional remedy map (diagnosis code to allowed intents) and the envelope margin. Absent, it changes no digest. | `Promotion` | `promotion` |
| Diagnosis catalog | The codes an executor may diagnose; `unknown` never becomes a rule. A decision's `diagnosis_code` must be one of them. | `ParseDiagnosisCodes`, `UnknownDiagnosis` | `cognition.executor.diagnosisCatalog` |
| Intent flags | `emergency` and `safetyFunction` mark an intent no rule may recommend, whatever its risk. | `Intent.Emergency`, `Intent.SafetyFunction` | `actions.intents[].emergency`, `.safetyFunction` |
| Playbook | A file of human-approved rules, compiled against a spec but sealed with its own digest, so the spec and policy digests never change with it. | `CompiledPlaybook`, `CompilePlaybookFile` | `<site>.playbook.yaml` |
| Rule (playbook) | A deterministic answer to one recurring, verified kind of case: a diagnosis code, a `when` over spec features, a recommended intent and preset, an envelope inside a cap, a pinned kind, a mode, a shadow exit, an audit rate and a rate cap. The compiler refuses a rule that recommends an R3+, emergency or safety-function intent. | `Rule`, `Recommendation`, `Range`, `PinnedKind` | `rules[]` |
| Registered schema | One immutable, digested event schema version stored for the event log to validate against. Re-registering identical bytes is allowed; different bytes are refused. `spec` is the only writer of the table. | `RegisterEventSchema` | `event_schemas` |

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| Config, manifest | Spec | A SituationSpec is compiled and digested; configuration is not. |
| Rule (for a spec expression) | Trigger, transition, or reducer | Each has its own semantics; a rule is a playbook rule. |
