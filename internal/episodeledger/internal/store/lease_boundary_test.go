package store_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestOwnerLeaseComparesAsTimeWhenOneSideHasNoFraction(t *testing.T) {
	t.Parallel()
	leaseUntil := at.Add(500 * time.Millisecond)
	for name, tc := range map[string]struct {
		now  time.Time
		want bool
	}{
		"whole second before a fractional lease": {at, true},
		"just before the lease":                  {leaseUntil.Add(-time.Nanosecond), true},
		"exactly at the lease":                   {leaseUntil, false},
		"after the lease":                        {at.Add(time.Second), false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db := storagetest.OpenTempWithoutForeignKeys(t)
			err := db.WithTx(t.Context(), func(raw *sql.Tx) error {
				if _, err := raw.ExecContext(t.Context(), "INSERT INTO runtime_owner (singleton_id, owner_epoch, owner_instance, acquired_at, heartbeat_at, lease_until) VALUES (1, 'e', 'i', ?, ?, ?)",
					kernel.FormatTime(at), kernel.FormatTime(at), kernel.FormatTime(leaseUntil)); err != nil {
					return err
				}
				held, err := store.Join(raw).OwnerHoldsLease(t.Context(), "e", tc.now)
				if err != nil || held != tc.want {
					t.Fatalf("held=%v err=%v, want %v", held, err, tc.want)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
