package domain

import (
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// Document is a decoded command, intent, decision or outcome JSON object. The
// original map is kept because canonical digests depend on its exact fields.
type Document map[string]any

// String returns a string field, or "" when absent or not a string.
func (d Document) String(key string) string {
	value, _ := d[key].(string)
	return value
}

// Int returns a numeric field as int, or 0 when absent.
func (d Document) Int(key string) int {
	value, _ := d[key].(float64)
	return int(value)
}

// Int64 returns a numeric field as int64, or 0 when absent.
func (d Document) Int64(key string) int64 {
	value, _ := d[key].(float64)
	return int64(value)
}

// Object returns an object field, or nil when absent.
func (d Document) Object(key string) map[string]any {
	value, _ := d[key].(map[string]any)
	return value
}

// Digest returns the raw bytes of a "sha256:" digest field, or nil when the
// field is absent or malformed.
func (d Document) Digest(key string) []byte {
	digest, err := canonicaljson.DecodeDigest(d.String(key))
	if err != nil {
		return nil
	}
	return digest
}

// verifyDigest reports whether digest is the canonical domain digest of document.
func verifyDigest(domain canonicaljson.Domain, document Document, digest []byte) bool {
	return canonicaljson.VerifySum(domain, map[string]any(document), digest)
}

// verifyIntentDigest reports whether digest binds the intent document, which
// excludes its own digest field from the hashed content.
func verifyIntentDigest(document Document, digest []byte) bool {
	if !canonicaljson.HasSumLength(digest) || !contractsv1.VerifyIntentDigest(document) {
		return false
	}
	expected, err := contractsv1.IntentDigest(document)
	if err != nil {
		return false
	}
	return expected == canonicaljson.EncodeDigest(digest)
}
