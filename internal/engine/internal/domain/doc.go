// Package domain holds the engine's rules as pure functions over loaded values:
// watermark arithmetic, persisted Situation state integrity and rebuild,
// version-write derivation, collision-free lineage identity, due-timer matching
// with boot fencing, and heartbeat timer identity. It performs no I/O and reads
// no clock.
package domain
