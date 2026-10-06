// Package runartifact exports a consistent, independently verifiable view of
// one Agentic Stream database run. It is an evidence projection, not a hot loop
// write path: the facade delegates to internal/app, which uses pure rules in
// internal/domain, read-only SQL in internal/store and the artifact directory
// in internal/transport. It owns no tables.
package runartifact
