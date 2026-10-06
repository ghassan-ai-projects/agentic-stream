// Package api provides the small, loopback-safe HTTP surface: liveness,
// readiness, epoch controls, approvals and the Server-Sent Events delivery of
// durable notifications. Business operations remain in internal packages. The
// facade exposes the handlers and their configuration; request rules, frame
// formats and problem mapping live in internal/domain and the net/http handlers
// in internal/transport.
package api
