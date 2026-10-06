// Package native implements the in-process Go episode executor.
//
// The executor owns the bounded model/tool loop. Providers only propose model
// output; tools are read-only capabilities and every Decision still returns to
// the episodes and policy packages for authoritative validation. The facade
// delegates to internal/app, which uses pure rules in internal/domain, the
// OpenAI-compatible provider in internal/transport and the event-log evidence
// tool in internal/store.
package native
