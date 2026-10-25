// Package transport is the plant wire host's connection edge: it accepts one
// node connection at a time from a listener the caller created owner-only,
// reads one record per line within the contract's largest record, and writes
// host records under a write deadline.
package transport
