package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// InsertEpisode inserts an admitted episode. The error wraps the database
// error; IsLiveEpisodeViolation classifies a live-episode uniqueness failure.
func (t *Tx) InsertEpisode(ctx context.Context, req domain.Admission, dispatchPolicy, acceptedAt string) error {
	if _, err := t.q.ExecContext(ctx, admitEpisodeSQL,
		req.EpisodeID, req.SchedulerItemID, req.TenantID, req.SituationID, req.SituationVersion,
		req.ExecutorName, req.ExecutorVersion, req.ModelPolicy, req.PromptVersion,
		req.SnapshotSHA256, req.PromptSHA256, req.ObjectiveSHA256, req.AdmissionKey, req.RequestJSON,
		acceptedAt, dispatchPolicy, req.PolicyEpoch,
	); err != nil {
		return fmt.Errorf("insert episode: %w", err)
	}
	return nil
}

// IsLiveEpisodeViolation reports whether err is the one-live-episode-per-
// situation uniqueness violation.
func IsLiveEpisodeViolation(err error) bool {
	return storage.IsUniqueViolation(err, "episodes.situation_id")
}

const admitEpisodeSQL = `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, prompt_sha256, objective_sha256, admission_key, request_json, lifecycle_status, accepted_at,
			dispatch_policy, policy_epoch
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'admitted', ?, ?, ?)`
