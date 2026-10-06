// Package app orders the engine's use cases: drain a partition or the global
// log, apply each record in one transaction, fire due processing-time timers,
// and rebuild in-memory Situations after a rollback. It owns the in-memory
// operator, Situation and cognition planes and the run mutex; it decides
// through domain and persists through store, and holds no SQL.
package app
