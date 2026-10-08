package episodeledger_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestSupersededAttemptOnlyAcceptsCurrentCancellation(t *testing.T) {
	db, err := storagetest.Open(t.Context(), filepath.Join(t.TempDir(), "cancel-fence.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	seedEpisode(t, t.Context(), db, "episode")
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	var identity episodeledger.Identity
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		var err error
		identity, err = episodeledger.StartAttempt(t.Context(), tx, "episode", "attempt", now)
		if err != nil {
			return err
		}
		return episodeledger.TransitionAttempt(t.Context(), tx, identity, episodeledger.AttemptRunning, now, nil)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE episodes SET lifecycle_status = 'superseded' WHERE episode_id = 'episode'"); err != nil {
		t.Fatal(err)
	}
	stale := identity
	stale.Fence--
	tests := []struct {
		name     string
		identity episodeledger.Identity
		to       episodeledger.AttemptStatus
		reason   episodeledger.RejectionReason
	}{
		{"stale cancellation", stale, episodeledger.AttemptCancelled, episodeledger.RejectStaleAttempt},
		{"current production", identity, episodeledger.AttemptProduced, episodeledger.RejectEpisodeClosed},
		{"current cancellation", identity, episodeledger.AttemptCancelled, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
				return episodeledger.TransitionAttempt(t.Context(), tx, tt.identity, tt.to, now, nil)
			})
			if tt.reason == "" && err != nil {
				t.Fatal(err)
			}
			if tt.reason != "" && !reasonIs(err, tt.reason) {
				t.Fatalf("transition = %v, want %s", err, tt.reason)
			}
		})
	}
}
