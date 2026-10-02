package episodeledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// ErrLiveEpisodeConflict means a reconsideration collides with a live episode.
var ErrLiveEpisodeConflict = errors.New("one live episode per situation constraint")

// Admission is the materialized durable episode input, independent of an executor.
type Admission struct {
	EpisodeID, SchedulerItemID, Kind, TenantID, SituationID                  string
	SituationVersion                                                         int
	ExecutorName, ExecutorVersion, ModelPolicy, PromptVersion                string
	SnapshotSHA256, PromptSHA256, ObjectiveSHA256, AdmissionKey, RequestJSON []byte
	DispatchPolicy, PolicyEpoch                                              string
}

// Admit admits the episode. A reconsideration that collides with a
// live episode for its Situation reports ErrLiveEpisodeConflict.
func Admit(ctx context.Context, tx *sql.Tx, req Admission, now time.Time) error {
	dispatchPolicy := admissionDispatchPolicy(req.DispatchPolicy)
	return persistAdmittedEpisode(ctx, tx, req, now, dispatchPolicy)
}

func admissionDispatchPolicy(declared string) string {
	// P8: an empty dispatch policy is SHADOW — nothing enters action
	// governance unless the spec declared active. The CHECK column stays
	// strict (active|shadow); this is the only place a value is written.
	if declared == "" {
		return "shadow"
	}
	return declared
}

func persistAdmittedEpisode(ctx context.Context, tx *sql.Tx, req Admission, now time.Time, dispatchPolicy string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, prompt_sha256, objective_sha256, admission_key, request_json, lifecycle_status, accepted_at,
			dispatch_policy, policy_epoch
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'admitted', ?, ?, ?)`,
		req.EpisodeID, req.SchedulerItemID, req.TenantID, req.SituationID, req.SituationVersion,
		req.ExecutorName, req.ExecutorVersion, req.ModelPolicy, req.PromptVersion,
		req.SnapshotSHA256, req.PromptSHA256, req.ObjectiveSHA256, req.AdmissionKey, req.RequestJSON,
		formatAcceptedAt(now),
		dispatchPolicy, req.PolicyEpoch,
	); err != nil {
		if req.Kind == "reconsider" && isLiveEpisodeConstraint(err) {
			return fmt.Errorf("insert episode: %w: %w", ErrLiveEpisodeConflict, err)
		}
		return fmt.Errorf("insert episode: %w", err)
	}
	return nil
}

func isLiveEpisodeConstraint(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) || sqliteErr.Code() != sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed: episodes.situation_id")
}
func formatAcceptedAt(value time.Time) string {
	// accepted_at is ordered as SQLite TEXT. Fixed-width nanoseconds keep the
	// durable lexical order identical to chronological order across writers.
	return value.UTC().Format("2006-01-02T15:04:05.000000000Z")
}
