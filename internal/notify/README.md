# Notify module

Notify owns the durable Channel-B notification outbox: tenant-local gapless
cursors, event deduplication with retention tombstones, bounded paged reads with
lag and poison handling, and the versioned lifecycle contract every producer's
event must satisfy. HTTP delivery lives in `api`.

Read [the vocabulary](UBIQUITOUS_LANGUAGE.md) before changing rules.

| Layer | Responsibility |
| --- | --- |
| Facade | `Append` and `AppendLifecycleEvent` (join the caller's transaction), `SourceForTenant`, `New(db)` → `Service.ReadPage`, `Service.Prune` |
| App | Seal → deduplicate → tombstone check → allocate → insert; lifecycle build and append; page read with resume refusal, audit and poison accounting; prune |
| Domain | Lifecycle contract (embedded JSON Schema plus tenant/authority binding), sealing, payload-conflict and tombstone decisions, resume/lag refusal, poison budget, retention floor, audit details |
| Store | The only SQL for `notifications`, `notification_cursors`, `notification_event_tombstones`, `notification_poison_attempts` and `notification_audits`; an opaque `Tx` that is the caller's transaction, one opened by `WithTx`, or autocommit |

The store has its own plain row types and does not import domain, which keeps the
module four levels deep (domain 2, store 2, app 3, facade 4) so producers can sit
at any level above it.

`New` requires the database. Appends have no dependencies and use the caller's
transaction so a state change and its notification commit together. A producer
passes a `LifecycleEvent` whose `Payload` is one of eight typed structs (the payload fixes
the event type); the domain stamps the tenant, the stable source and the source authority, seals
the digest and validates the contract before anything is stored, so an event that violates
the contract never reaches the outbox.

Reads check the limit, then the tenant's retained bounds: a cursor before the
oldest retained notification, or a subscriber further behind than `MaxLag`, is
refused and audited. A record whose digest, JSON or envelope fails is poison: it
fails the page until its three-attempt budget is spent, then it is skipped with
an audit row in the same transaction that clears its counter.

`Prune` is the retention operation (seven-day floor, tombstones for the
deduplication horizon). It is not scheduled by the runtime; see the open
decision in the migration record.

[Migration record](../../docs/notify-reference-module-2026-10-06/README.md).
Architecture gates enforce a pure domain, no SQL or `database/sql` in app, exact
write ownership of the five tables, opaque store types and facade delegation.
