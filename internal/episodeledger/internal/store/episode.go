package store

import (
	"context"
	"fmt"
)

// RebindEpisode persists the validated live snapshot and consumes one rebind.
func (t *Tx) RebindEpisode(ctx context.Context, episodeID string, version int, digest, request []byte) error {
	return t.write(ctx, `UPDATE episodes SET situation_version = ?, snapshot_sha256 = ?, request_json = ?,
 stale_rebind_count = stale_rebind_count + 1 WHERE episode_id = ?`, version, digest, request, episodeID)
}

// BindRequest persists the request after a fenced attempt identity is bound.
func (t *Tx) BindRequest(ctx context.Context, episodeID string, request []byte) error {
	return t.write(ctx, `UPDATE episodes SET request_json = ? WHERE episode_id = ?`, request, episodeID)
}

// AbandonRebind quarantines an invalid live snapshot and consumes one rebind.
func (t *Tx) AbandonRebind(ctx context.Context, episodeID, now string, terminal []byte) error {
	return t.write(ctx, `UPDATE episodes SET lifecycle_status = 'abandoned', ended_at = ?, terminal_json = ?,
 stale_rebind_count = stale_rebind_count + 1 WHERE episode_id = ?`, now, terminal, episodeID)
}

// AbandonEpisode records a terminal quarantine outcome.
func (t *Tx) AbandonEpisode(ctx context.Context, episodeID, now string, terminal []byte) error {
	return t.write(ctx, `UPDATE episodes SET lifecycle_status = 'abandoned', ended_at = ?, terminal_json = ? WHERE episode_id = ?`, now, terminal, episodeID)
}

// ConcludeEpisode records the terminal execution outcome.
func (t *Tx) ConcludeEpisode(ctx context.Context, episodeID, now string, terminal []byte) error {
	return t.write(ctx, `UPDATE episodes SET lifecycle_status = 'concluded', ended_at = ?, terminal_json = ? WHERE episode_id = ?`, now, terminal, episodeID)
}

// RetainEpisodeForRetry returns the episode to running for the next bounded attempt.
func (t *Tx) RetainEpisodeForRetry(ctx context.Context, episodeID string) error {
	return t.write(ctx, `UPDATE episodes SET lifecycle_status = 'running', ended_at = NULL, terminal_json = NULL WHERE episode_id = ?`, episodeID)
}

// SupersedeEpochEpisodes cancels the in-flight episodes of a killed epoch.
func (t *Tx) SupersedeEpochEpisodes(ctx context.Context, epoch, now string) error {
	if _, err := t.q.ExecContext(ctx, supersedeEpochEpisodesSQL, now, epoch); err != nil {
		return fmt.Errorf("supersede in-flight episodes of killed epoch: %w", err)
	}
	return nil
}

var supersedeEpochEpisodesSQL = `
		UPDATE episodes SET lifecycle_status = 'superseded', ended_at = ?
		WHERE policy_epoch = ? AND lifecycle_status IN ` + liveLifecycles

func (t *Tx) write(ctx context.Context, query string, args ...any) error {
	if _, err := t.q.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("write episode: %w", err)
	}
	return nil
}
