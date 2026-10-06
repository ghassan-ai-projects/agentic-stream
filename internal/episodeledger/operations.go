package episodeledger

import (
	"context"
	"database/sql"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

// Admit admits the episode. A reconsideration that collides with a live
// episode for its Situation reports ErrLiveEpisodeConflict.
func Admit(ctx context.Context, tx *sql.Tx, req Admission, now time.Time) error {
	return app.Admit(ctx, store.Join(tx), req, now)
}

// StartAttempt allocates the next fence for an admitted episode and records a
// dispatched worker attempt on the caller's transaction.
func StartAttempt(ctx context.Context, tx *sql.Tx, episodeID, attemptID string, now time.Time) (Identity, error) {
	return app.StartAttempt(ctx, store.Join(tx), episodeID, attemptID, now)
}

// StartAttemptOwned allocates an attempt fenced to the current runtime epoch.
// Live composition uses this entry point.
func StartAttemptOwned(ctx context.Context, tx *sql.Tx, episodeID, attemptID, ownerEpoch string, now time.Time) (Identity, error) {
	return app.StartAttemptOwned(ctx, store.Join(tx), episodeID, attemptID, ownerEpoch, now)
}

// TransitionAttempt applies a valid attempt transition and records terminal
// data. The identity is checked, at the wall clock, before the state mutation.
func TransitionAttempt(ctx context.Context, tx *sql.Tx, identity Identity, to AttemptStatus, now time.Time, terminalJSON []byte) error {
	return app.TransitionAttempt(ctx, store.Join(tx), identity, to, now, terminalJSON, time.Now().UTC())
}

// RecordRejection durably records a rejected worker input or Decision. It is
// idempotent for the same identity, reason, details, and timestamp.
func RecordRejection(ctx context.Context, tx *sql.Tx, identity Identity, reason RejectionReason, detailsJSON []byte, now time.Time) error {
	return app.RecordRejection(ctx, store.Join(tx), identity, reason, detailsJSON, now)
}

// RecoverUnfinishedAttempts abandons active attempts owned by an older or
// missing runtime epoch. When costs is not nil, reservations of permanently
// abandoned episodes are released; requeued episodes keep theirs.
func RecoverUnfinishedAttempts(ctx context.Context, tx *sql.Tx, currentEpoch string, now time.Time, costs CostSettler) (RecoveryReport, error) {
	return app.RecoverUnfinishedAttempts(ctx, store.Join(tx), currentEpoch, now, costs)
}
