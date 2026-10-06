// Package transport owns replay's file and database resources: trace file
// reading, isolated database creation, trace ingestion through the ingress
// adapter, and the per-run directory for repeated replays. It sequences
// nothing and validates no evidence.
package transport
