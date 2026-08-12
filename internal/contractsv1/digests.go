package contractsv1

import "github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"

// IntentDigest computes the digest carried by an Intent. The digest field is
// excluded from its own preimage so the contract is not self-referential.
func IntentDigest(document map[string]any) (string, error) {
	withoutDigest := make(map[string]any, len(document))
	for key, value := range document {
		if key != "intent_digest" {
			withoutDigest[key] = value
		}
	}
	return canonicaljson.Digest(canonicaljson.DomainIntent, withoutDigest)
}

// VerifyIntentDigest verifies the digest carried by an Intent in constant
// time against the digest of the document without its digest field.
func VerifyIntentDigest(document map[string]any) bool {
	provided, ok := document["intent_digest"].(string)
	if !ok {
		return false
	}
	expected, err := IntentDigest(document)
	if err != nil || len(expected) != len(provided) {
		return false
	}
	return canonicaljson.Verify(canonicaljson.DomainIntent, withoutIntentDigest(document), provided)
}

func withoutIntentDigest(document map[string]any) map[string]any {
	withoutDigest := make(map[string]any, len(document))
	for key, value := range document {
		if key != "intent_digest" {
			withoutDigest[key] = value
		}
	}
	return withoutDigest
}
