// Package domain holds notification vocabulary and rules as pure functions:
// the versioned lifecycle contract, event sealing, deduplication, tombstone,
// resume, poison and retention decisions, and audit details. It performs no
// I/O and reads no clock; time arrives as a parameter.
package domain
