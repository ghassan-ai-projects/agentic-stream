# Event log language

Names mean the same thing in conversation, code, storage and audit trails.

## Terms

| Term | Meaning | Code name | Storage name |
| --- | --- | --- | --- |
| Evidence log | Append-only normalized event store, the only executable evidence | facade `EventLog` | `event_log` |
| Log position | Durable monotonic offset of one record | `domain.LogPosition` | `position` |
| Record | One stored normalized event | `domain.Record` | `event_log` row |
| Evidence admission | Envelope contract plus schema validation before an append | `admit` steps in app | — |
| Registered schema | Active JSON Schema for an event type/version | `domain.EventSchema` | `event_schemas` row |
| Quarantine | Durable record of an invalid event, bounded retries | app quarantine use case | `event_quarantine` row |
| Quarantine identity | Stable `q_` id derived from event id and payload bytes | `domain.QuarantineIdentity` | `quarantine_id` |
| Hash conflict | Same event id quarantined with different payload bytes | `domain` digest compare | `event_id_hash_conflict` |
| Overflow | Retries spent; the loss is recorded as a gap | app overflow step | `quarantine_retry_exhausted` |
| Release | Operator marks a quarantined event eligible for redrive | app release use case | `status='released'` |
| Redrive | Validate and append a released event atomically, once | app redrive use case | `redriven_at` |
| Gap | Durable record that a quarantined event exhausted its retries; never deletes evidence | `InsertOverflowGap` (store) | `event_gaps` row |
| Entity window | Read-only bounded evidence window over one entity | `domain.EntityWindow` | `event_log` range scan |
| Unit of work | One transaction spanning admission, insert and lifecycle steps | `store.Unit` | — |

## Retired words

| Retired | Replacement | Why |
| --- | --- | --- |
| `storedEvent` | `domain.StoredRecord` | Named after meaning (scanned columns before decode), not storage accident |
| `eventBody` | `domain.EncodedEvent` | Says what it is: the encoded payload columns |
| `admit` (ambiguous) | evidence admission | Avoids collision with episode admission |
