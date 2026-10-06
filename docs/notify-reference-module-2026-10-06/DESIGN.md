# Design

## Layers

| Layer | Responsibility | Must not |
| --- | --- | --- |
| Facade `internal/notify` (4) | Aliases of domain values, `New(db)`, one-line delegation, `store.Join` of the caller's `*sql.Tx` | Hold logic, SQL or transactions |
| `internal/app` (3) | Use cases in order: validate → seal → dedupe/tombstone → allocate → insert → audit; read page; poison accounting; prune | Import `database/sql` or `storage` |
| `internal/domain` (2) | Lifecycle contract, sealing, dedupe/tombstone/resume/poison/retention decisions, audit details, record decoding | Do I/O or read a clock |
| `internal/store` (2) | Every SQL statement; opaque `Tx` that may be the caller's transaction or autocommit | Decide anything |

The store does not import domain: it has its own plain row types and the app maps
between them. That keeps the whole module at four levels, so only five importers
move in the layer table (see PLAN).

## Public API

| Operation | Purpose |
| --- | --- |
| `Append(ctx, tx, event, now)` | Validate and append a CloudEvent in the caller's transaction |
| `AppendLifecycleEvent(ctx, tx, LifecycleEvent)` | Build, seal, contract-validate and append a lifecycle event |
| `SourceForTenant(tenant)` | Stable lifecycle source |
| `New(db)` → `Service` | Database is required (constructor error otherwise) |
| `Service.ReadPage(ctx, PageRequest, now)` | Cursor-resumable page with lag/poison handling |
| `Service.Prune(ctx, now, retention)` | Retention with tombstones; seven-day floor |
| `Type*`, `Err*`, `Record`, `Page`, `PageRequest`, `LifecycleEvent` | Vocabulary |

## Rules every operation follows

- A state change and its audit commit together (poison skip: audit + counter clear in one transaction).
- Result sets are read and closed before any write (single-connection databases).
- Clock reads stay with callers; `now` is a parameter everywhere.
- Error precedence is unchanged (see PLAN).

## Enforcement

Architecture gates: layer table and import allowlists, pure domain, no SQL/`database/sql`
in app, SQL only in the store, table ownership moved to `internal/notify/internal/store`,
facade delegation, opaque store `Store`/`Tx`.

## Schema or wire changes

None. The contract JSON files move to `internal/notify/internal/domain/contracts/`
(content unchanged). Documentation links are updated.
