// Package remote adapts the out-of-process EpisodeWorker protocol to the
// episode Executor port: it maps durable requests to the wire, issues the
// per-attempt evidence capability, accounts the streamed budget, and accepts a
// Decision only with a matching terminal.
package remote
