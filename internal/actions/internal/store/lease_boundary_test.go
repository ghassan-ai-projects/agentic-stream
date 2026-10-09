package store

import (
	"testing"
	"time"
)

func TestLeaseLivenessComparesAsTimeAcrossWholeSecondAndFraction(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	until := now.Add(500 * time.Millisecond)
	var outboxID int64
	inTx(t, db, func(tx *Tx) error {
		c, _, err := tx.NextCandidate(t.Context(), now)
		outboxID = c.OutboxID
		if err != nil {
			return err
		}
		_, err = tx.AcquireLease(t.Context(), outboxID, "owner", until, now)
		return err
	})
	inTx(t, db, func(tx *Tx) error {
		for at, want := range map[time.Time]bool{now: true, until.Add(-time.Nanosecond): true, until: false, until.Add(time.Second): false} {
			if live, err := tx.LeaseIsLive(t.Context(), outboxID, "owner", at); err != nil || live != want {
				t.Fatalf("live at %v = %v, err=%v, want %v", at, live, err, want)
			}
		}
		return nil
	})
}
