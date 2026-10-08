package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
)

const episodeRecordSQL = `
	SELECT episode_id, scheduler_item_id, situation_id, situation_version, executor_name, executor_version, model_policy,
		prompt_version, COALESCE(dispatch_policy, ''), snapshot_sha256, lifecycle_status, COALESCE(current_attempt_id, ''), current_fence,
		accepted_at, COALESCE(ended_at, ''), terminal_json
	FROM episodes WHERE tenant_id = ? AND episode_id = ?`

// Episode reads one of the tenant's episodes with its attempts and refused
// results.
func (t *Tx) Episode(ctx context.Context, tenantID, episodeID string) (domain.EpisodeRecord, error) {
	var r domain.EpisodeRecord
	var snapshot, terminal []byte
	if err := t.q.QueryRowContext(ctx, episodeRecordSQL, tenantID, episodeID).Scan(&r.EpisodeID, &r.SchedulerItemID, &r.SituationID, &r.SituationVersion, &r.ExecutorName, &r.ExecutorVersion, &r.ModelPolicy,
		&r.PromptVersion, &r.DispatchPolicy, &snapshot, &r.LifecycleStatus, &r.CurrentAttemptID, &r.CurrentFence, &r.AcceptedAt, &r.EndedAt, &terminal); err != nil {
		return domain.EpisodeRecord{}, fmt.Errorf("read episode %s: %w", episodeID, err)
	}
	r.SnapshotSHA256, r.Terminal = "sha256:"+hex.EncodeToString(snapshot), rawOrNil(terminal)
	var err error
	if r.Attempts, err = t.attempts(ctx, episodeID); err != nil {
		return domain.EpisodeRecord{}, err
	}
	r.Rejections, err = t.Rejections(ctx, episodeID)
	return r, err
}

func (t *Tx) attempts(ctx context.Context, episodeID string) ([]domain.AttemptView, error) {
	rows, err := t.q.QueryContext(ctx, `SELECT attempt_id, fence, status, started_at, COALESCE(ended_at, ''), terminal_json
		FROM episode_attempts WHERE episode_id = ? ORDER BY fence`, episodeID)
	if err != nil {
		return nil, fmt.Errorf("read episode attempts: %w", err)
	}
	return scanAttemptViews(rows)
}

func scanAttemptViews(rows *sql.Rows) ([]domain.AttemptView, error) {
	defer func() { _ = rows.Close() }()
	attempts := []domain.AttemptView{}
	for rows.Next() {
		var attempt domain.AttemptView
		var terminal []byte
		if err := rows.Scan(&attempt.AttemptID, &attempt.Fence, &attempt.Status, &attempt.StartedAt, &attempt.EndedAt, &terminal); err != nil {
			return nil, fmt.Errorf("scan episode attempt: %w", err)
		}
		attempt.Terminal = rawOrNil(terminal)
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err() //nolint:wrapcheck // The iteration error is the driver's.
}

// rawOrNil keeps a stored JSON document, or nothing when none was stored.
func rawOrNil(document []byte) []byte {
	if len(document) == 0 {
		return nil
	}
	return document
}
