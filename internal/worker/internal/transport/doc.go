// Package transport owns the runtime's private Unix sockets for the worker
// protocol: it listens on the evidence socket and dials an EpisodeWorker. The
// protocol rules live in domain; a fake worker for tests lives in
// internal/testsupport/workerfake.
package transport
