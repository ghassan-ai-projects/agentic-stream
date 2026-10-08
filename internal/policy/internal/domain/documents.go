package domain

import (
	"strings"
	"unicode"
)

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
