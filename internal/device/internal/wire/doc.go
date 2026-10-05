// Package wire encodes and decodes device records: one canonical,
// newline-delimited JSON object per frame, at most MaxFrameBytes, validated
// against the schema of its message type, then parsed into domain values.
// Original documents are retained for digests and evidence. It decides nothing beyond
// well-formedness.
package wire
