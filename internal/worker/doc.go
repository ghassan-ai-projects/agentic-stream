// Package worker implements the runtime-facing side of the versioned episode
// worker protocol. It owns wire validation and event sequencing; a worker
// handler only supplies bounded, typed proposals. The facade exposes the
// protocol constants, the budget and socket-path rules, the reference Server and
// the socket helpers; the rules live in internal/domain and the gRPC server and
// sockets in internal/transport.
package worker
