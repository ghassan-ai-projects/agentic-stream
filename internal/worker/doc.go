// Package worker implements the runtime-facing side of the versioned episode
// worker protocol: the protocol constants, the budget and socket-path rules,
// the private evidence listener and the worker dialer. The rules live in
// internal/domain and the sockets in internal/transport. The runtime is the
// protocol's client; a validating fake worker for tests lives in
// internal/testsupport/workerfake.
package worker
