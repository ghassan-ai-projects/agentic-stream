package replay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"time"
)

func loadShadowInput(ctx context.Context, db *storage.DB, item replayItem, tenantID, specDigest, policyDigest string, evaluationTime time.Time) (ShadowInput, error) {
	var snapshot, persistedDigest []byte
	if err := db.QueryRowContext(ctx, `
		SELECT snapshot_json, snapshot_sha256 FROM situation_versions
		WHERE situation_id = ? AND version = ?`, item.SituationID, item.SituationVersion).Scan(&snapshot, &persistedDigest); err != nil {
		return ShadowInput{}, fmt.Errorf("load shadow snapshot: %w", err)
	}
	canonical, err := verifiedShadowSnapshot(snapshot, persistedDigest)
	if err != nil {
		return ShadowInput{}, err
	}
	return newShadowInput(item, tenantID, specDigest, policyDigest, canonical, evaluationTime), nil
}

func verifiedShadowSnapshot(snapshot, persistedDigest []byte) ([]byte, error) {
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
	if err := verifyShadowSnapshotDigest(document, persistedDigest); err != nil {
		return nil, err
	}
	return canonical, nil
}

func verifyShadowSnapshotDigest(document map[string]any, persistedDigest []byte) error {
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

func newShadowInput(item replayItem, tenantID, specDigest, policyDigest string, canonical []byte, evaluationTime time.Time) ShadowInput {
	return ShadowInput{
		TenantID: tenantID, EpisodeKey: replayEpisodeKey(item.SituationID, item.SituationVersion, item.TriggerID),
		EpisodeID: item.EpisodeID, SituationID: item.SituationID, SituationVersion: item.SituationVersion,
		TriggerID: item.TriggerID, AttemptID: "shadow-attempt/" + item.EpisodeID, Fence: 1,
		SnapshotDigest: item.SnapshotDigest, SpecDigest: specDigest, PolicyDigest: policyDigest,
		SnapshotJSON: append([]byte(nil), canonical...), EvaluationTime: evaluationTime,
	}
}

func shadowEntityID(snapshot []byte) (string, error) {
	var document struct {
		Entity struct {
			ID string `json:"id"`
		} `json:"entity"`
	}
	if err := json.Unmarshal(snapshot, &document); err != nil {
		return "", fmt.Errorf("decode shadow entity: %w", err)
	}
	if document.Entity.ID == "" {
		return "", fmt.Errorf("shadow snapshot entity id is required")
	}
	return document.Entity.ID, nil
}

func cloneShadowInput(input ShadowInput) ShadowInput {
	input.SnapshotJSON = append([]byte(nil), input.SnapshotJSON...)
	return input
}
