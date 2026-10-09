package store

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/domain"
)

func TestConditionRoundTripsAndInsertIsIdempotent(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	inTx(t, s, func(tx *Tx) error {
		if _, found, err := tx.LoadCondition(t.Context(), "w-1"); err != nil || found {
			t.Fatalf("missing watch found=%v err=%v", found, err)
		}
		for range 2 {
			if err := tx.InsertCondition(t.Context(), "w-1", condition(), testNow); err != nil {
				return err
			}
		}
		stored, found, err := tx.LoadCondition(t.Context(), "w-1")
		if err != nil || !found || stored != condition() {
			t.Fatalf("stored = %+v found=%v err=%v", stored, found, err)
		}
		return nil
	})
}

func TestFireIsRecordedOncePerEventAndSpendsAllowance(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	inTx(t, s, func(tx *Tx) error { return tx.InsertCondition(t.Context(), "w-1", condition(), testNow) })
	inTx(t, s, func(tx *Tx) error {
		first, err := tx.RecordFire(t.Context(), "w-1", "evt-1", testNow)
		again, againErr := tx.RecordFire(t.Context(), "w-1", "evt-1", testNow)
		if err != nil || againErr != nil || !first || again {
			t.Fatalf("first=%v again=%v errs=%v %v", first, again, err, againErr)
		}
		return tx.SpendAllowance(t.Context(), "w-1", testNow)
	})
	inTx(t, s, func(tx *Tx) error {
		if recorded, err := tx.RecordFire(t.Context(), "w-1", "evt-2", testNow); err != nil || !recorded {
			t.Fatalf("second event recorded=%v err=%v", recorded, err)
		}
		if err := tx.SpendAllowance(t.Context(), "w-1", testNow); err != nil {
			return err
		}
		if recorded, err := tx.RecordFire(t.Context(), "w-1", "evt-3", testNow); err != nil || recorded {
			t.Fatalf("exhausted watch recorded=%v err=%v", recorded, err)
		}
		_, found, err := tx.LoadActive(t.Context(), "w-1", testNow)
		if err != nil || found {
			t.Fatalf("disabled watch still active: found=%v err=%v", found, err)
		}
		return nil
	})
}

func TestExpiryHidesWatchesFromActiveReads(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	inTx(t, s, func(tx *Tx) error { return tx.InsertCondition(t.Context(), "w-1", condition(), testNow) })
	if candidates, err := s.ActiveForTarget(t.Context(), "motor-1", testNow); err != nil || len(candidates) != 1 || candidates[0] != (domain.Candidate{WatchID: "w-1", SituationID: "sit-1"}) {
		t.Fatalf("candidates = %+v err=%v", candidates, err)
	}
	if candidates, err := s.ActiveForTarget(t.Context(), "other", testNow); err != nil || len(candidates) != 0 {
		t.Fatalf("other target candidates = %+v err=%v", candidates, err)
	}
	later := testNow.Add(2 * time.Hour)
	inTx(t, s, func(tx *Tx) error {
		if _, found, err := tx.LoadActive(t.Context(), "w-1", later); err != nil || found {
			t.Fatalf("expired watch active before marking: found=%v err=%v", found, err)
		}
		return tx.ExpireDue(t.Context(), later)
	})
	inTx(t, s, func(tx *Tx) error {
		if recorded, err := tx.RecordFire(t.Context(), "w-1", "evt-1", testNow); err != nil || recorded {
			t.Fatalf("expired watch recorded a fire: %v %v", recorded, err)
		}
		return nil
	})
}

func TestLoadConditionFailsClosedOnUnreadableStoredExpiry(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	inTx(t, s, func(tx *Tx) error { return tx.InsertCondition(t.Context(), "w-1", condition(), testNow) })
	inTx(t, s, func(tx *Tx) error {
		if _, err := tx.tx.ExecContext(t.Context(), "UPDATE watch_conditions SET expires_at = 'soon' WHERE watch_id = 'w-1'"); err != nil {
			return err
		}
		if _, found, err := tx.LoadCondition(t.Context(), "w-1"); err == nil || found {
			t.Fatalf("unreadable expiry found=%v err=%v, want an error", found, err)
		}
		return nil
	})
}

func TestUnreadableStoredExpiryIsExpiredNotActive(t *testing.T) {
	t.Parallel()
	for name, corrupt := range map[string]string{
		"garbage":         "soon",
		"offset":          "2999-01-01T00:00:00.000000000+02:00",
		"space separated": "2999-01-01 00:00:00.000000000Z",
		"digits only":     "9999-99-99T99:99:99.999999999Z",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := openStore(t)
			inTx(t, s, func(tx *Tx) error { return tx.InsertCondition(t.Context(), "w-1", condition(), testNow) })
			inTx(t, s, func(tx *Tx) error {
				if _, err := tx.tx.ExecContext(t.Context(), "UPDATE watch_conditions SET expires_at = ? WHERE watch_id = 'w-1'", corrupt); err != nil {
					return err
				}
				if _, found, err := tx.LoadActive(t.Context(), "w-1", testNow); err != nil || found {
					t.Fatalf("LoadActive found=%v err=%v, want hidden", found, err)
				}
				if recorded, err := tx.RecordFire(t.Context(), "w-1", "evt-1", testNow); err != nil || recorded {
					t.Fatalf("RecordFire recorded=%v err=%v, want refused", recorded, err)
				}
				return tx.ExpireDue(t.Context(), testNow)
			})
			if candidates, err := s.ActiveForTarget(t.Context(), "motor-1", testNow); err != nil || len(candidates) != 0 {
				t.Fatalf("ActiveForTarget = %+v err=%v, want none", candidates, err)
			}
			var status string
			inTx(t, s, func(tx *Tx) error {
				return tx.tx.QueryRowContext(t.Context(), "SELECT status FROM watch_conditions WHERE watch_id = 'w-1'").Scan(&status)
			})
			if status != "expired" {
				t.Fatalf("status = %q, want expired", status)
			}
		})
	}
}

func TestAWatchIsVisibleUpToItsExpiryAndHiddenAtIt(t *testing.T) {
	t.Parallel()
	expiry := condition().ExpiresAt
	cases := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"a nanosecond before", expiry.Add(-time.Nanosecond), true},
		{"at the expiry instant", expiry, false},
		{"after", expiry.Add(time.Second), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := openStore(t)
			inTx(t, s, func(tx *Tx) error { return tx.InsertCondition(t.Context(), "w-1", condition(), testNow) })
			candidates, err := s.ActiveForTarget(t.Context(), "motor-1", tc.at)
			if visible := len(candidates) == 1; err != nil || visible != tc.want {
				t.Errorf("ActiveForTarget visible = %v, %v; want %v", visible, err, tc.want)
			}
			inTx(t, s, func(tx *Tx) error {
				if _, found, err := tx.LoadActive(t.Context(), "w-1", tc.at); err != nil || found != tc.want {
					t.Errorf("LoadActive found = %v, %v; want %v", found, err, tc.want)
				}
				if recorded, err := tx.RecordFire(t.Context(), "w-1", "evt-1", tc.at); err != nil || recorded != tc.want {
					t.Errorf("RecordFire recorded = %v, %v; want %v", recorded, err, tc.want)
				}
				return nil
			})
		})
	}
}
