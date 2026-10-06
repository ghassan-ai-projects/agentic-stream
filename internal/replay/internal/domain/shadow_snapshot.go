package domain

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// VerifiedSnapshot returns the canonical snapshot document only when it is
// valid against the snapshot contract and matches its persisted digest.
func VerifiedSnapshot(snapshot, persistedDigest []byte) ([]byte, error) {
	canonical, err := canonicaljson.Marshal(json.RawMessage(snapshot))
	if err != nil {
		return nil, fmt.Errorf("canonicalize shadow snapshot: %w", err)
	}
	var document map[string]any
	if err := json.Unmarshal(canonical, &document); err != nil {
		return nil, fmt.Errorf("decode shadow snapshot: %w", err)
	}
	if err := contractsv1.Validate(contractsv1.SchemaSnapshot, document); err != nil {
		return nil, fmt.Errorf("validate shadow snapshot: %w", err)
	}
	if err := verifySnapshotDigest(document, persistedDigest); err != nil {
		return nil, err
	}
	return canonical, nil
}

func verifySnapshotDigest(document map[string]any, persistedDigest []byte) error {
	digest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, document)
	if err != nil {
		return fmt.Errorf("digest shadow snapshot: %w", err)
	}
	decoded, err := canonicaljson.DecodeDigest(digest)
	if err != nil || !bytes.Equal(decoded, persistedDigest) {
		return fmt.Errorf("shadow snapshot digest mismatch")
	}
	return nil
}
