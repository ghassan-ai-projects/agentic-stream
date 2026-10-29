package episodeledger

import (
	"context"
	"database/sql"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

// EpisodeFence is what the ledger records about an episode: its tenant,
// lifecycle, current attempt and fence. CheckOpenIdentity and CheckIdentity
// judge a worker identity against it.
type EpisodeFence = domain.EpisodeFence

// ReadEpisodeFence reads an episode's fence on the caller's transaction;
// found is false for an unknown episode.
func ReadEpisodeFence(ctx context.Context, tx *sql.Tx, episodeID string) (EpisodeFence, bool, error) {
	return app.ReadEpisodeFence(ctx, store.Join(tx), episodeID)
}

// ReadAttemptStatus reads the status of the attempt under the identity on the
// caller's transaction; a missing attempt is refused as the wrong attempt.
func ReadAttemptStatus(ctx context.Context, tx *sql.Tx, identity Identity) (AttemptStatus, error) {
	return app.ReadAttemptStatus(ctx, store.Join(tx), identity)
}

// ReadEpisodeLifecycle reads an episode's lifecycle status from a database
// handle, for a poll outside any transaction; an unknown episode is an error.
func ReadEpisodeLifecycle(ctx context.Context, db *sql.DB, episodeID string) (LifecycleStatus, error) {
	return app.ReadEpisodeLifecycle(ctx, store.Reader(db), episodeID)
}

// DispatchableEpisode is an admitted episode as the dispatcher sees it: its
// admission record and how often it was re-bound to a newer Situation version.
type DispatchableEpisode = domain.DispatchableEpisode

// NextDispatchableEpisode reads the tenant's oldest admitted or running episode
// on the caller's transaction; found is false when nothing is dispatchable.
// With killedSuperseded set it also admits superseded episodes without an
// attempt under a killed policy epoch, so the dispatcher can quarantine them.
func NextDispatchableEpisode(ctx context.Context, tx *sql.Tx, tenantID string, killedSuperseded bool) (DispatchableEpisode, bool, error) {
	return app.NextDispatchableEpisode(ctx, store.Join(tx), tenantID, killedSuperseded)
}

// ReadAdmission reads the admission record of an episode from a database
// handle; an unknown episode is an error.
func ReadAdmission(ctx context.Context, db *sql.DB, episodeID string) (Admission, error) {
	return app.ReadAdmission(ctx, store.Reader(db), episodeID)
}

// ReasoningInFlight reports from a database handle whether reasoning is in
// flight: an episode is admitted or running, or a scheduler item that may start
// at once is still pending. An item held back by a debounce or cooldown window
// that has not opened at now is not reasoning in flight; now is the time of the
// clock that times scheduling, the one the admitter polls the queue at.
func ReasoningInFlight(ctx context.Context, db *sql.DB, now time.Time) (bool, error) {
	return app.ReasoningInFlight(ctx, store.Reader(db), now)
}
