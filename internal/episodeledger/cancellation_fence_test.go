package episodeledger

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestSupersededAttemptOnlyAcceptsCurrentCancellation(t *testing.T) {
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "cancel-fence.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	seedEpisode(t, t.Context(), db, "episode")
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	var identity Identity
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		var err error
		identity, err = StartAttempt(t.Context(), tx, "episode", "attempt", now)
		if err != nil {
			return err
		}
		return TransitionAttempt(t.Context(), tx, identity, AttemptRunning, now, nil)
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
		identity Identity
		to       AttemptStatus
		reason   RejectionReason
	}{
		{"stale cancellation", stale, AttemptCancelled, RejectStaleAttempt},
		{"current production", identity, AttemptProduced, RejectEpisodeClosed},
		{"current cancellation", identity, AttemptCancelled, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return TransitionAttempt(t.Context(), tx, tt.identity, tt.to, now, nil) })
			if tt.reason == "" && err != nil {
				t.Fatal(err)
			}
			if tt.reason != "" && !IsIdentityReason(err, tt.reason) {
				t.Fatalf("transition = %v, want %s", err, tt.reason)
			}
		})
	}
}
