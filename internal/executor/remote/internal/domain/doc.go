// Package domain holds the remote executor's rules as pure functions over
// protocol values: mapping a durable request to the wire request, handshake and
// evidence-configuration checks, event-by-event stream validation, and trusted
// budget accounting. It performs no I/O and reads no clock.
package domain
