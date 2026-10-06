package domain

import (
	"bytes"
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
	var snapshot map[string]any
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	if err := contractsv1.Validate(contractsv1.SchemaSnapshot, snapshot); err != nil {
		return nil, fmt.Errorf("validate snapshot: %w", err)
	}
	if SnapshotString(snapshot, "situation_id") != situationID ||
		SnapshotInt(snapshot, "situation_version") != version ||
		SnapshotString(snapshot, "tenant_id") != tenantID {
		return nil, fmt.Errorf("snapshot identity does not match episode admission")
	}
	return bindSnapshotEvidence(snapshotJSON, persistedDigest, traceparent, tracestate, snapshot)
}

func bindSnapshotEvidence(snapshotJSON, persistedDigest []byte, traceparent, tracestate string, snapshot map[string]any) (*SnapshotEvidence, error) {
	entityID, err := SnapshotEntityID(snapshotJSON)
	if err != nil {
		return nil, fmt.Errorf("load snapshot entity: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, snapshot)
	if err != nil {
		return nil, fmt.Errorf("digest snapshot: %w", err)
	}
	decodedDigest, err := canonicaljson.DecodeDigest(digest)
	if err != nil || !bytes.Equal(decodedDigest, persistedDigest) {
		return nil, fmt.Errorf("snapshot digest does not match persisted situation version")
	}
	return &SnapshotEvidence{
		Document: snapshot, EntityID: entityID, Digest: digest, Traceparent: traceparent, Tracestate: tracestate,
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

func SnapshotString(snapshot map[string]any, key string) string {
	value, _ := snapshot[key].(string)
	return value
}

func SnapshotInt(snapshot map[string]any, key string) int {
	value, _ := snapshot[key].(float64)
	return int(value)
}
