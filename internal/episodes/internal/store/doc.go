// Package store holds every episodes SQL statement. The module's use cases
// are transaction-scoped (the public API takes the caller's *sql.Tx, like
// episodeledger), so store methods receive that transaction and decide
// nothing: they select rows and insert records named after domain actions.
package store
