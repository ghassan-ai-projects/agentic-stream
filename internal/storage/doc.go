// Package storage provides SQLite persistence for the runtime. The facade
// exposes the database handle, the openers and the busy-retry and row helpers;
// the persistence rules live in internal/domain and the SQLite adapter in
// internal/store. It owns only the schema_migrations table.
package storage
