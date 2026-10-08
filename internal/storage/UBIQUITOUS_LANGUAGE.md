# Storage ubiquitous language

| Term | Meaning | Code name | Storage name |
| --- | --- | --- | --- |
| Database | The SQLite database in WAL mode. Opening runs pending migrations. | `DB`, `Open` | file path |
| Fresh database | A database path atomically reserved for an isolated replay, so an existing file or symlink is never mistaken for a disposable one. | `OpenFresh` | — |
| Migration | An embedded, ordered, applied-once schema step. | `Migrate` | `schema_migrations`, `migrations/*.sql` |
| Unit of work | A transaction that commits when the callback returns nil. State change and audit commit together. | `WithTx` | — |
| Busy retry | Retrying a transaction after transient SQLite writer contention. The callback must roll back before returning. | `RetrySQLiteBusy`, `IsSQLiteBusy` | `SQLITE_BUSY` |
| Checkpoint | A non-blocking WAL checkpoint; leftover frames are normal. | `Checkpoint` | WAL |
| Stored time check | The SQL function every opened database carries: `stored_time_ok(text)` is 1 only for the exact text `kernel.FormatTime` writes. A lease or expiry compared as text pairs its comparison with it, so an unreadable value is expired, never live. | `stored_time_ok` | SQL function |
| Row collection | Scans every remaining row in order; iteration failures are reported by name. | `CollectRows` | — |

`storage` owns `schema_migrations` and nothing else. Every other table has one
owning module (see the durable-owner map in the architecture gates).
