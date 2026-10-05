// Package store owns the engine's SQL (checkpoints, inbox, operator state,
// Situations, versions, lineage, timers) and the owner fence on the original
// transaction. It selects and writes data and decodes stored bytes; every
// decision lives in domain and app.
package store
