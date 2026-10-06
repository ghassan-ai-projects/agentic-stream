// Package domain holds the interlock rules as pure code: the two statuses, the
// tripped sentinel, validation of a status change and the fail-closed readiness
// check. It performs no I/O and reads no clock.
package domain
