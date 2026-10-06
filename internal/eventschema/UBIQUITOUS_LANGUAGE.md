# Event schema ubiquitous language

| Term | Meaning | Code name | Storage name |
| --- | --- | --- | --- |
| Definition | A schema bound to one normalized event type and schema version, with its payload fields keyed by name. | `Definition` | `registry_data.json` |
| Field | One payload field exposed to deterministic operators: path, type, unit, optionality and allowed values. | `Field` | `Definition.Fields` |
| Schema reference | The name a spec input uses to select a definition. | `Lookup(ref)` | `schemaRef` in the spec |
| Registry | The embedded catalog of definitions (motor, sensor, pump, pond, bay). Domain data, never a Go literal. | `Lookup` | `registry_data.json` |
| Registered schema | One immutable, digested schema version stored for the event log to validate against. Re-registering identical bytes is allowed; different bytes are refused. | `Register` | `event_schemas` |
| Active schema | A registered schema the event log accepts new events against. | status `active` | `event_schemas.status` |

The registry is pure data; `Register` is the only writer of `event_schemas` and
runs inside the caller's transaction (the spec deployment). The event log reads
the table when it appends.
