// Package plantwire hosts plant wire sessions: a node (a simulated world or an
// edge gateway) dials the host's owner-only Unix socket, announces its boards
// and kinds, and the host admits it against the vendored contract and the
// commissioned kind files, answers its barriers and keeps it alive with
// heartbeats. It is an adapter beside internal/device behind Agentic Stream's
// existing ports and holds no device or domain knowledge. It is a thin facade
// over internal/app (host and session use cases), internal/domain (pure
// rules), internal/wire (the vendored contract and record codec) and
// internal/transport (the connection edge). See the [module guide](README.md).
package plantwire
