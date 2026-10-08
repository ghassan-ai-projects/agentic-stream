# Ubiquitous language

| Term | Meaning | Code name | Storage / wire name |
| --- | --- | --- | --- |
| Notification | One durable Channel-B CloudEvent addressed to a tenant | `domain.Record` | `notifications` row |
| Cursor | Tenant-local, gapless, monotonic position of a notification | `Cursor` | `notifications.cursor` |
| Highwater | Next cursor to allocate for a tenant | `NextCursor` | `notification_cursors.next_cursor` |
| Page | Bounded delivery result; `NextCursor` advances past skipped poison rows | `domain.Page` | — |
| Lifecycle event | A Channel-B event of one of the eight pinned v1 types | `LifecycleEvent` + `Payload` | `io.agenticstream.*.v1` |
| Event identity | The event id, subject and partition key a payload always receives, so one fact yields one event from every producer | `ApprovalWithdrawnEvent`, `OutcomeRecordedEvent`, ... | `approval.withdrawn:<approval>`, `approval/<approval>` |
| Lifecycle contract | Schema + tenant/authority binding every lifecycle event satisfies | `domain.ValidateLifecycle` | `notification-contract-v1.json` |
| Source authority | The tenant's stable CloudEvents source | `SourceForTenant` | `//agentic-stream/tenant/<id>` |
| Seal | Canonical JSON and SHA-256 of an event | `domain.Seal` | `event_json`, `event_sha256` |
| Tombstone | Retired event identity kept for the deduplication horizon | `Tombstone` | `notification_event_tombstones` |
| Retention floor | Minimum retention, seven days | `RetentionFloor` | — |
| Poison | A stored notification that fails digest, decoding or validation | `ErrNotificationPoison` | `notification_poison_attempts` |
| Poison budget | Attempts before an audited skip, three | `PoisonBudget` | `attempts` |
| Resume refusal | Expired cursor or too-slow subscriber, each audited | `ResumeRefusal` | `notification_audits.action` |

## Retired words

| Retired | Replacement | Why |
| --- | --- | --- |
| `notifycontract` (package) | `notify/internal/domain` lifecycle contract | One owner; the Go package was never importable outside the module |
| `AppendLifecycleEventWithTrace` | `AppendLifecycleEvent(LifecycleEvent)` | The trace is part of the request, not a variant |
| `Known` / `Types` | `lifecycleTypes` (private) | One ordered list; `Types` was test-only |
| `Type*` facade constants, `LifecycleEvent.Type`, `Data map[string]any` | `Payload` (typed, `EventType()`) | The payload fixes the type and its fields; producers no longer repeat `tenant_id`/`source_authority` |
