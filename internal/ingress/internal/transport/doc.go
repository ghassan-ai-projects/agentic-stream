// Package transport frames lines from trace files and Unix-socket clients and
// owns the socket's listener safety, accept loop, client registry and shutdown
// ordering. It makes no admission or quarantine decision.
package transport
