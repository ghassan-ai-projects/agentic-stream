package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

// DispatchedEpisode is the oldest dispatchable episode as scanned, before its
// request document is hydrated.
type DispatchedEpisode struct {
	EpisodeID        string
	SchedulerItemID  string
	TenantID         string
	SituationID      string
	SituationVersion int
	ExecutorName     string
	ExecutorVersion  string
	ModelPolicy      string
	PromptVersion    string
	SnapshotSHA256   []byte
	PromptSHA256     []byte
	ObjectiveSHA256  []byte
	AdmissionKey     []byte
	RequestJSON      []byte
	DispatchPolicy   string
	PolicyEpoch      string
	StaleRebindCount int
}

// DispatchableEpisode reads the oldest admitted or running episode inside the
// caller's transaction. With killedSuperseded set it also admits superseded
// episodes without an attempt under a killed policy epoch, so the runner can
// release their reservation and quarantine them. It returns sql.ErrNoRows
// when nothing is dispatchable.
func DispatchableEpisode(ctx context.Context, tx *Tx, tenantID string, killedSuperseded bool) (DispatchedEpisode, error) {
	var episode DispatchedEpisode
	err := tx.tx.QueryRowContext(ctx, dispatchableEpisodeQuery(killedSuperseded), tenantID).Scan(
		&episode.EpisodeID, &episode.SchedulerItemID, &episode.TenantID, &episode.SituationID, &episode.SituationVersion,
		&episode.ExecutorName, &episode.ExecutorVersion, &episode.ModelPolicy, &episode.PromptVersion,
		&episode.SnapshotSHA256, &episode.PromptSHA256, &episode.ObjectiveSHA256, &episode.AdmissionKey, &episode.RequestJSON,
		&episode.DispatchPolicy, &episode.PolicyEpoch, &episode.StaleRebindCount,
	)
	if err != nil {
		return DispatchedEpisode{}, err //nolint:wrapcheck // Caller distinguishes no rows and preserves query error context.
	}
	return episode, nil
}

var (
	liveEpisodePredicate = "lifecycle_status IN " + episodeledger.LifecycleSQL(episodeledger.LifecycleStatus.Live)

	killedUnstartedEpisodePredicate = ` OR (lifecycle_status = '` + string(episodeledger.LifecycleSuperseded) + `' AND current_attempt_id IS NULL
			AND EXISTS (SELECT 1 FROM epoch_control WHERE epoch = episodes.policy_epoch AND state = 'killed'))`
)

func dispatchableEpisodeQuery(killedSuperseded bool) string {
	lifecyclePredicate := liveEpisodePredicate
	if killedSuperseded {
		lifecyclePredicate += killedUnstartedEpisodePredicate
	}
	return fmt.Sprintf(`
		SELECT episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
		       executor_name, executor_version, model_policy, prompt_version,
		       snapshot_sha256, prompt_sha256, objective_sha256, admission_key, request_json,
		       dispatch_policy, policy_epoch, stale_rebind_count
		FROM episodes
		WHERE tenant_id = ? AND (%s)
		ORDER BY accepted_at, episode_id LIMIT 1`, lifecyclePredicate)
}
