package domain

import (
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// SnapshotEvidence is the validated immutable Situation snapshot an episode
// reasons over: the decoded document, the bound entity and the
// verified digest.
type SnapshotEvidence struct {
	Document    map[string]any
	EntityID    string
	Digest      string
	Traceparent string
	Tracestate  string
}

// ValidateSnapshotEvidence decodes a stored snapshot, checks it against the
// snapshot contract and the episode admission identity, and verifies its
// persisted digest. A stale or tampered snapshot never reaches a worker.
func ValidateSnapshotEvidence(snapshotJSON, persistedDigest []byte, traceparent, tracestate, situationID string, version int, tenantID string) (*SnapshotEvidence, error) {
	snapshot, err := contractsv1.DecodeDocument(snapshotJSON, contractsv1.SchemaSnapshot)
	if err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	if contractsv1.DocumentString(snapshot, "situation_id") != situationID ||
		contractsv1.DocumentInt(snapshot, "situation_version") != version ||
		contractsv1.DocumentString(snapshot, "tenant_id") != tenantID {
		return nil, fmt.Errorf("snapshot identity does not match episode admission")
	}
	return bindSnapshotEvidence(snapshotJSON, persistedDigest, traceparent, tracestate, snapshot)
}

func bindSnapshotEvidence(snapshotJSON, persistedDigest []byte, traceparent, tracestate string, snapshot map[string]any) (*SnapshotEvidence, error) {
	entityID, err := SnapshotEntityID(snapshotJSON)
	if err != nil {
		return nil, fmt.Errorf("load snapshot entity: %w", err)
	}
	if !contractsv1.VerifyDocumentDigest(canonicaljson.DomainSnapshot, snapshot, persistedDigest) {
		return nil, fmt.Errorf("snapshot digest does not match persisted situation version")
	}
	return &SnapshotEvidence{
		Document: snapshot, EntityID: entityID, Digest: canonicaljson.EncodeDigest(persistedDigest), Traceparent: traceparent, Tracestate: tracestate,
	}, nil
}

// SnapshotEntityID reads the bound entity identity out of a snapshot.
func SnapshotEntityID(raw []byte) (string, error) {
	var snapshot struct {
		Entity struct {
			ID string `json:"id"`
		} `json:"entity"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return "", fmt.Errorf("decode snapshot entity: %w", err)
	}
	if snapshot.Entity.ID == "" {
		return "", fmt.Errorf("snapshot entity id is required")
	}
	return snapshot.Entity.ID, nil
}
