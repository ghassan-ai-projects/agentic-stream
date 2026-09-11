# A-046 · `internal/storage/storage.go`

LOC: 195 · Audit date: 2026-09-11 · Verdict: FINDINGS

## Bar (close only when every line is true)
- No swallowed errors; every failure path reports a cause.
- Transactions roll back on error; migrations apply atomically with their schema_migrations row.
- Storage is SQLite WAL via modernc.org/sqlite; embedded migrations match the schema the code queries.
- Replay reservation cannot be bypassed by an existing database, symlink, or concurrent creator.

## Findings
- **[LOW] F1. hasMigrationsTable swallows query errors and misreads them as "table missing"** — `internal/storage/storage.go:145-153`. Any `QueryRowContext` failure (I/O error, corrupt DB, cancelled context) returns `false` with no error, so `Migrate` proceeds to run migrations against a database whose state it could not inspect and fails later with a confusing `INSERT INTO schema_migrations` error. Return the error (only `sql.ErrNoRows` should mean "no table").

## Checked, not an issue
- P1: `WithTx` and `runMigration` use the deferred-rollback pattern; commit errors wrapped `%w`; contexts honored throughout.
- P2: `OpenFresh` reserves the path atomically (`O_EXCL` file + `Mkdir` reservation), rejects pre-existing `-wal`/`-shm` sidecars, and cleans up the reservation on every failure path; `Open` refuses a live replay reservation.
- P4: modernc.org/sqlite with `_pragma=journal_mode(WAL)`, `busy_timeout(30000)`, `synchronous(NORMAL)`, `wal_autocheckpoint(256)`, `_txlock=immediate` — matches the documented stack; embedded migrations verified against the columns the storage/eventlog/engine code writes (incl. `tracestate` from 007, state columns from 009, lifecycle columns from 003).
- P5: exported symbols documented; `context.Context` first param.
- P6: `storage_test.go` pins WAL/synchronous/busy-timeout per pooled connection, migration application, lifecycle-status mapping, and Open idempotence.
- P7: reservation release in `Close` keeps first-error semantics and never masks the DB close error.
