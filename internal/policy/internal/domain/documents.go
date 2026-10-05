package domain

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

func CanonicalDocumentMatches(raw, digest []byte, domain canonicaljson.Domain) bool {
	var document map[string]any
	if json.Unmarshal(raw, &document) != nil || len(digest) != sha256.Size {
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
		return subtle.ConstantTimeCompare([]byte(expected), []byte("sha256:"+hex.EncodeToString(digest))) == 1
	}
	return canonicaljson.Verify(domain, document, "sha256:"+hex.EncodeToString(digest))
}

func NormalizedTarget(intentID string, document map[string]any) string {
	parameters, _ := document["parameters"].(map[string]any)
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

func ContainsControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func DocumentString(document map[string]any, key string) string {
	value, _ := document[key].(string)
	return value
}

func DocumentInt(document map[string]any, key string) int {
	value, _ := document[key].(float64)
	return int(value)
}

func FormatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
