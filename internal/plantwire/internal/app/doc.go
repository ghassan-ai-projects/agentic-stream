// Package app holds the plant wire host's use cases: commissioning the kind
// files, accepting one node session at a time, and running a session: the
// handshake, a heartbeat every interval, every barrier answered at once, a
// rebirth asked on a gap, and the end of the session on a node death or a lost
// connection, after which the next session is accepted.
package app
