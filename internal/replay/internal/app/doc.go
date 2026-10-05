// Package app sequences replay sessions: compile spec, save deployment,
// derive the virtual clock epoch from the trace, ingest, run the engine,
// materialize episodes, collect the canonical result, and run the requested
// capability phase. It reaches files, sockets and SQL only through the
// transport and store layers.
package app
