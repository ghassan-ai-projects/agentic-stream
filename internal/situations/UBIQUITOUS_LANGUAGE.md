# Situations ubiquitous language

| Term | Meaning | Code name | Storage name |
| --- | --- | --- | --- |
| Situation | The durable, versioned judgement about one entity: phase, severity, confidence, facts and evidence. The mutable current state of one occurrence. | `Situation` | `situations` (owned by `engine`) |
| Occurrence | One open-to-close episode of a situation for an entity; reopening needs a cooldown. | `Situation.OccurrenceID` | `situations.occurrence_id` |
| Version | An immutable record of one Situation change. Every change publishes a new version. | `Version` | `situation_versions` (owned by `engine`) |
| Phase | The named lifecycle state, with a severity; some are terminal. | `Situation.Phase` | `phase` |
| Transition | A guarded move between phases, taken when its condition has held. | spec `transitions`, `Situation.ConditionStart` | — |
| Reducer | Folds features into facts: `latest_event_time`, `set_union`. | `reducers.go` | spec `reducers` |
| Facts | The reduced values of a situation. | `Situation.Facts` | `situations.state_json` (canonical reducer state) |
| Evidence | The identifiers of the events and features a situation rests on. | `Situation.Evidence` | `lineage_sets` (owned by `engine`) |
| Engine | Applies features to situations for one partition and keeps its in-memory index. | `Engine`, `ApplyFeature`, `Restore`, `Reset` | — |
| CEL features | The `features` view shared with cognition: facts and evidence with every missing operator output defaulted. | `CELFeatures` | — |

## Retired words

| Avoid | Use | Why |
| --- | --- | --- |
| Incident, alert | Situation | A situation is a versioned judgement, not a notification. |
| State machine (for the package) | Situation engine | The phase machine is one part; the engine also reduces facts and versions. |
