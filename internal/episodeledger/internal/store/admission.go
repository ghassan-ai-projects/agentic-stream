package store

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const admissionColumns = `episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, prompt_sha256, objective_sha256, admission_key, request_json,
			dispatch_policy, policy_epoch`

var admitEpisodeSQL = `
		INSERT INTO episodes (` + admissionColumns + `, lifecycle_status, accepted_at)
		VALUES (` + strings.Join(slices.Repeat([]string{"?"}, len(admissionValues(domain.Admission{}))), ", ") + `, 'admitted', ?)`

func admissionValues(a domain.Admission) []any {
	return []any{
		a.EpisodeID, a.SchedulerItemID, a.TenantID, a.SituationID, a.SituationVersion,
		a.ExecutorName, a.ExecutorVersion, a.ModelPolicy, a.PromptVersion,
		a.SnapshotSHA256, a.PromptSHA256, a.ObjectiveSHA256, a.AdmissionKey, a.RequestJSON,
		a.DispatchPolicy, a.PolicyEpoch,
	}
}

func admissionTargets(a *domain.Admission) []any {
	return []any{
		&a.EpisodeID, &a.SchedulerItemID, &a.TenantID, &a.SituationID, &a.SituationVersion,
		&a.ExecutorName, &a.ExecutorVersion, &a.ModelPolicy, &a.PromptVersion,
		&a.SnapshotSHA256, &a.PromptSHA256, &a.ObjectiveSHA256, &a.AdmissionKey, &a.RequestJSON,
		&a.DispatchPolicy, &a.PolicyEpoch,
	}
}

// InsertEpisode inserts an admitted episode. The error wraps the database
// error; IsLiveEpisodeViolation classifies a live-episode uniqueness failure.
func (t *Tx) InsertEpisode(ctx context.Context, req domain.Admission, acceptedAt time.Time) error {
	if _, err := t.q.ExecContext(ctx, admitEpisodeSQL, append(admissionValues(req), kernel.FormatTime(acceptedAt))...); err != nil {
		return fmt.Errorf("insert episode: %w", err)
	}
	return nil
}

// ReadAdmission reads the admission record of an episode; an unknown episode
// is an error.
func (t *Tx) ReadAdmission(ctx context.Context, episodeID string) (domain.Admission, error) {
	var admission domain.Admission
	err := t.q.QueryRowContext(ctx, `SELECT `+admissionColumns+` FROM episodes WHERE episode_id = ?`, episodeID).Scan(admissionTargets(&admission)...)
	if err != nil {
		return domain.Admission{}, fmt.Errorf("load episode admission %s: %w", episodeID, err)
	}
	return admission, nil
}

// IsLiveEpisodeViolation reports whether err is the one-live-episode-per-
// situation uniqueness violation.
func IsLiveEpisodeViolation(err error) bool {
	return storage.IsUniqueViolation(err, "episodes.situation_id")
}
