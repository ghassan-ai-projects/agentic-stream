// Package interlock provides the read-only action readiness boundary: the durable
// global switch that blocks the action plane. The facade exposes the reader, the
// tripped sentinel and the setter; the rules live in internal/domain and the
// `runtime_interlock` SQL in internal/store.
package interlock
