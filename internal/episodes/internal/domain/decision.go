package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
)

// DecisionIDFromJSON reads a decision document's declared identity.
func DecisionIDFromJSON(raw []byte) string {
	var document struct {
		DecisionID string `json:"decision_id"`
	}
	if json.Unmarshal(raw, &document) != nil {
		return ""
	}
	return document.DecisionID
}

// StorageDecisionDigest returns the digest to persist for a decision: the
// contract digest when the document carries one, else the raw bytes' hash.
// The boolean reports whether a contract digest was present.
func StorageDecisionDigest(raw []byte) ([]byte, bool) {
	digest, hasContractDigest := decisionDigestForStorage(raw)
	if !hasContractDigest {
		rawHash := sha256.Sum256(raw)
		digest = rawHash[:]
	}
	return digest, hasContractDigest
}

func decisionDigestForStorage(raw []byte) ([]byte, bool) {
	canonical, err := canonicaljson.Marshal(json.RawMessage(raw))
	if err != nil {
		return nil, false
	}
	var document map[string]any
	if err := json.Unmarshal(canonical, &document); err != nil {
		return nil, false
	}
	return DocumentDigestForStorage(document)
}

// DocumentDigestForStorage digests a decision document over the decision
// domain, reporting whether the digest could be derived.
func DocumentDigestForStorage(document map[string]any) ([]byte, bool) {
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, document)
	if err != nil {
		return nil, false
	}
	decoded, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		return nil, false
	}
	return decoded, true
}

// ValidationFailureJSON describes a rejected Decision. Without a contract
// digest the raw bytes' hash is recorded so the rejected input stays
// traceable.
func ValidationFailureJSON(validationErr error, raw []byte, hasContractDigest bool) ([]byte, error) {
	var validationJSON []byte
	var err error
	var typed *decisions.ValidationError
	if errors.As(validationErr, &typed) {
		validationJSON, err = json.Marshal(map[string]any{"reason": typed.Reason, "details": typed.Details})
	} else {
		validationJSON, err = json.Marshal(map[string]any{"reason": "schema_invalid", "details": validationErr.Error()})
	}
	if err != nil {
		return nil, fmt.Errorf("marshal decision validation: %w", err)
	}
	if hasContractDigest {
		return validationJSON, nil
	}
	return rawValidationFailureJSON(validationJSON, raw)
}

func rawValidationFailureJSON(validationJSON, raw []byte) ([]byte, error) {
	var err error
	rawHash := sha256.Sum256(raw)
	var details map[string]any
	if err := json.Unmarshal(validationJSON, &details); err != nil {
		return nil, fmt.Errorf("decode decision validation: %w", err)
	}
	details["raw_sha256"] = hex.EncodeToString(rawHash[:])
	validationJSON, err = json.Marshal(details)
	if err != nil {
		return nil, fmt.Errorf("marshal raw decision validation: %w", err)
	}
	return validationJSON, nil
}
