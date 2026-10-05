// Package app orders the watch use cases: install a watch idempotently, fire
// matching watches once per event, and expire due watches under writer
// contention. It decides through domain and persists through store; it holds
// no SQL.
package app
