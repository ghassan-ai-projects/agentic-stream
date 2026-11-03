package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

var (
	liveEpisodePredicate = "lifecycle_status IN " + domain.LifecycleSQL(domain.LifecycleStatus.Live)

	reasoningInFlightSQL = `SELECT 1 FROM episodes WHERE ` + liveEpisodePredicate + `
		UNION ALL SELECT 1 FROM scheduler_items WHERE status = 'pending' AND not_before IS NULL LIMIT 1`

	killedUnstartedEpisodePredicate = ` OR (lifecycle_status = '` + string(domain.LifecycleSuperseded) + `' AND current_attempt_id IS NULL
			AND EXISTS (SELECT 1 FROM epoch_control WHERE epoch = episodes.policy_epoch AND state = 'killed'))`
)

func (t *Tx) NextDispatchableEpisode(ctx context.Context, tenantID string, killedSuperseded bool) (domain.DispatchableEpisode, bool, error) {
	var episode domain.DispatchableEpisode
	targets := append(admissionTargets(&episode.Admission), &episode.StaleRebindCount)
	err := t.q.QueryRowContext(ctx, dispatchableEpisodeQuery(killedSuperseded), tenantID).Scan(targets...)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DispatchableEpisode{}, false, nil
	}
	if err != nil {
		return domain.DispatchableEpisode{}, false, fmt.Errorf("load dispatchable episode: %w", err)
	}
	return episode, true, nil
}

func dispatchableEpisodeQuery(killedSuperseded bool) string {
	lifecyclePredicate := liveEpisodePredicate
	if killedSuperseded {
		lifecyclePredicate += killedUnstartedEpisodePredicate
	}
	return `SELECT ` + admissionColumns + `, stale_rebind_count FROM episodes
		WHERE tenant_id = ? AND (` + lifecyclePredicate + `)
		ORDER BY accepted_at, episode_id LIMIT 1`
}

func (t *Tx) ReasoningInFlight(ctx context.Context) (bool, error) {
	_, found, err := storage.QueryOptional[int](ctx, t.q, reasoningInFlightSQL)
	if err != nil {
		return false, fmt.Errorf("look for reasoning in flight: %w", err)
	}
	return found, nil
}
