# Storage ubiquitous language

| Term | Meaning | Code name | Storage name |
| --- | --- | --- | --- |
| Database | The SQLite database in WAL mode. Opening runs pending migrations. | `DB`, `Open` | file path |
| Fresh database | A database path atomically reserved for an isolated replay, so an existing file or symlink is never mistaken for a disposable one. | `OpenFresh` | — |
| Migration | An embedded, ordered, applied-once schema step. | `Migrate` | `schema_migrations`, `migrations/*.sql` |
| Unit of work | A transaction that commits when the callback returns nil. State change and audit commit together. | `WithTx` | — |
| Busy retry | Retrying a transaction after transient SQLite writer contention. The callback must roll back before returning. | `RetrySQLiteBusy` | `SQLITE_BUSY` |
| Checkpoint | A non-blocking WAL checkpoint; leftover frames are normal. | `Checkpoint` | WAL |
| Stored time check | The SQL function every opened database carries: `stored_time_ok(text)` is 1 only for the exact text `kernel.FormatTime` writes. A lease or expiry compared as text pairs its comparison with it, so an unreadable value is expired, never live. | `stored_time_ok` | SQL function |
| Row collection | Scans every remaining row in order; iteration failures are reported by name. | `CollectRows` | — |
| Query helper | Reads that every store would otherwise repeat: `QueryAll` runs a query and collects every row (a query failure is `run query: <cause>`); `QueryOptional` reads the first column of the first row and reports `found` instead of an error for no rows. Callers add the operation. | `QueryAll`, `QueryOptional`, `Querier` | — |
| Write fence | A check that runs on the caller's transaction and fails when the given epoch no longer owns the runtime. | `OwnerCheck` | — |
| Statement helpers | Small SQL value rules: an empty string is NULL, a boolean is 0 or 1, a changed-row count is zero when the driver cannot report it, and `IN (?, ...)` binds only values (the column is a literal identifier of the caller's SQL). | `NullIfEmpty`, `BoolInt`, `RowsAffected`, `InClause` | — |
| Unique violation | A SQLite uniqueness failure on `table.column`, whether from a primary key or a unique index. | `IsUniqueViolation` | `UNIQUE constraint failed` |
| Test database | A migrated temporary database for tests, copied from a template built once per migration set so a test skips the migration replay. Test code only. | `storagetest.OpenTemp`, `storagetest.Open` | — |

`storage` owns `schema_migrations` and nothing else. Every other table has one
owning module (see the durable-owner map in the architecture gates).
