package domain

import (
	"crypto/sha256"
	"crypto/subtle"
	"strings"
	"time"
	"unicode"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// DocumentDigestMatches verifies the original decoded document, including the intent self-digest.
func DocumentDigestMatches(document map[string]any, digest []byte, domain canonicaljson.Domain) bool {
	if len(digest) != sha256.Size {
		return false
	}
	if domain == canonicaljson.DomainIntent {
		if !contractsv1.VerifyIntentDigest(document) {
			return false
		}
		expected, err := contractsv1.IntentDigest(document)
		if err != nil {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(expected), []byte(canonicaljson.EncodeDigest(digest))) == 1
	}
	return canonicaljson.Verify(domain, document, canonicaljson.EncodeDigest(digest))
}

// NormalizedTarget resolves target before entity identity, retaining the intent fallback.
func NormalizedTarget(intentID string, parameters map[string]any) string {
	for _, key := range []string{"target", "entity_id"} {
		candidate, ok := parameters[key].(string)
		if !ok {
			continue
		}
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || len(candidate) > 256 || ContainsControl(candidate) {
			return intentID
		}
		return candidate
	}
	return intentID
}

// ContainsControl detects forbidden control characters in target identities.
func ContainsControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// FormatTime formats durable timestamps in UTC with nanosecond precision.
func FormatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
