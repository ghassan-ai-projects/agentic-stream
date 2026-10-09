package domain

import (
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// VerifiedSnapshot returns the canonical snapshot document only when it is
// valid against the snapshot contract and matches its persisted digest.
func VerifiedSnapshot(snapshot, persistedDigest []byte) ([]byte, error) {
	document, err := contractsv1.VerifyStoredDocument(contractsv1.SchemaSnapshot, canonicaljson.DomainSnapshot, snapshot, persistedDigest)
	if err != nil {
		return nil, fmt.Errorf("verify shadow snapshot: %w", err)
	}
	canonical, err := canonicaljson.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("canonicalize shadow snapshot: %w", err)
	}
	return canonical, nil
}
