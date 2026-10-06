// Package domain holds the episode worker protocol as pure rules: version and
// feature constants, size limits, the handshake and request checks, the event
// stream validator and the wall-time budget rule. Errors carry gRPC status codes
// because the codes are part of the protocol; the package opens no sockets and
// reads no clock.
package domain
