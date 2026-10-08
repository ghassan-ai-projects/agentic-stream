package episodeledger

import (
	"context"
	"database/sql"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
)

// Rebind persists the validated live snapshot and consumes one rebind.
func Rebind(ctx context.Context, tx *sql.Tx, episodeID string, version int, digest, request []byte) error {
	return app.Rebind(ctx, store.Join(tx), episodeID, version, digest, request)
}

// BindRequest persists the request after a fenced attempt identity is bound.
func BindRequest(ctx context.Context, tx *sql.Tx, episodeID string, request []byte) error {
	return app.BindRequest(ctx, store.Join(tx), episodeID, request)
}

// AbandonRebind quarantines an invalid live snapshot and consumes one rebind.
func AbandonRebind(ctx context.Context, tx *sql.Tx, episodeID string, now time.Time, terminal []byte) error {
	return app.AbandonRebind(ctx, store.Join(tx), episodeID, now, terminal)
}

// Abandon records a terminal quarantine outcome.
func Abandon(ctx context.Context, tx *sql.Tx, episodeID string, now time.Time, terminal []byte) error {
	return app.Abandon(ctx, store.Join(tx), episodeID, now, terminal)
}

// Conclude records the terminal execution outcome.
func Conclude(ctx context.Context, tx *sql.Tx, episodeID string, now time.Time, terminal []byte) error {
	return app.Conclude(ctx, store.Join(tx), episodeID, now, terminal)
}

// RetainForRetry retains the episode for the next bounded attempt.
func RetainForRetry(ctx context.Context, tx *sql.Tx, episodeID string) error {
	return app.RetainForRetry(ctx, store.Join(tx), episodeID)
}

// SupersedeEpoch cancels in-flight episodes of a killed epoch.
func SupersedeEpoch(ctx context.Context, tx *sql.Tx, epoch string, now time.Time) error {
	return app.SupersedeEpoch(ctx, store.Join(tx), epoch, now)
}

// SupersedeCoalesced cancels episodes and attempts bound to coalesced scheduler items.
func SupersedeCoalesced(ctx context.Context, tx *sql.Tx, situationID string, now time.Time) error {
	return app.SupersedeCoalesced(ctx, store.Join(tx), situationID, now)
}
