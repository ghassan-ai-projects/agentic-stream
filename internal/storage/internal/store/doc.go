// Package store is the SQLite adapter: it opens and migrates the runtime
// database, runs transactions and busy retries, checkpoints the WAL, reserves
// fresh databases for isolated replay and scans result rows. The rules it
// follows (connection string, backoff schedule, pending migrations, reserved
// names) live in domain.
package store
