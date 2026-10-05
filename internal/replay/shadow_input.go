package replay

import (
	"context"
	"fmt"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func loadShadowInput(ctx context.Context, db *storage.DB, episode domain.ReplayEpisode, tenantID, specDigest, policyDigest string, evaluationTime time.Time) (ShadowInput, error) {
	var snapshot, persistedDigest []byte
	if err := db.QueryRowContext(ctx, `
		SELECT snapshot_json, snapshot_sha256 FROM situation_versions
		WHERE situation_id = ? AND version = ?`, episode.SituationID, episode.SituationVersion).Scan(&snapshot, &persistedDigest); err != nil {
		return ShadowInput{}, fmt.Errorf("load shadow snapshot: %w", err)
	}
	canonical, err := domain.VerifiedSnapshot(snapshot, persistedDigest)
	if err != nil {
		return ShadowInput{}, err
	}
	return newShadowInput(episode, tenantID, specDigest, policyDigest, canonical, evaluationTime), nil
}

func newShadowInput(episode domain.ReplayEpisode, tenantID, specDigest, policyDigest string, canonical []byte, evaluationTime time.Time) ShadowInput {
	return ShadowInput{
		TenantID: tenantID, EpisodeKey: episode.EpisodeKey,
		EpisodeID: episode.EpisodeID, SituationID: episode.SituationID, SituationVersion: episode.SituationVersion,
		TriggerID: episode.TriggerID, AttemptID: "shadow-attempt/" + episode.EpisodeID, Fence: 1,
		SnapshotDigest: episode.SnapshotDigest, SpecDigest: specDigest, PolicyDigest: policyDigest,
		SnapshotJSON: append([]byte(nil), canonical...), EvaluationTime: evaluationTime,
	}
}
