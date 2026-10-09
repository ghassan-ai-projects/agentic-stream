package store

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
)

func leaseNext(t *testing.T, s Store, owner string, until, now time.Time) int64 {
	t.Helper()
	var outboxID int64
	if err := s.WithTx(t.Context(), func(tx *Tx) error {
		candidate, found, err := tx.NextCandidate(t.Context(), now)
		if err != nil || !found {
			t.Fatalf("candidate found=%v err=%v", found, err)
		}
		outboxID = candidate.OutboxID
		acquired, err := tx.AcquireLease(t.Context(), outboxID, owner, until, now)
		if err != nil || !acquired {
			t.Fatalf("acquire acquired=%v err=%v", acquired, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return outboxID
}

func TestALeaseIsExclusiveUntilItExpires(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	until := testNow.Add(time.Minute)
	outboxID := leaseNext(t, newStore(db), "first", until, testNow)
	cases := []struct {
		name  string
		owner string
		now   time.Time
		want  bool
	}{
		{"another dispatcher while the lease is live", "second", testNow, false},
		{"another dispatcher one nanosecond before expiry", "second", until.Add(-time.Nanosecond), false},
		{"another dispatcher at expiry", "second", until, true},
	}
	for _, tc := range cases {
		inTx(t, db, func(tx *Tx) error {
			if got, err := tx.AcquireLease(t.Context(), outboxID, tc.owner, tc.now.Add(time.Minute), tc.now); err != nil || got != tc.want {
				t.Fatalf("%s: acquired = %v, err = %v, want %v", tc.name, got, err, tc.want)
			}
			return nil
		})
	}
}

func TestOnlyTheLeaseHolderRefreshesItAndLivenessFollowsOwnership(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	outboxID := leaseNext(t, newStore(db), "owner", testNow.Add(time.Minute), testNow)
	inTx(t, db, func(tx *Tx) error {
		if ok, err := tx.RefreshLease(t.Context(), outboxID, "owner", testNow.Add(time.Hour), testNow); err != nil || !ok {
			t.Fatalf("holder refresh ok=%v err=%v", ok, err)
		}
		if ok, err := tx.RefreshLease(t.Context(), outboxID, "thief", testNow.Add(time.Hour), testNow); err != nil || ok {
			t.Fatalf("foreign refresh ok=%v err=%v, want refused", ok, err)
		}
		lease, found, err := tx.LoadOutboxLease(t.Context(), outboxID)
		if held, live := lease.LeaseStanding("owner", testNow); err != nil || !found || !held || !live {
			t.Fatalf("lease=%+v found=%v held=%v live=%v err=%v, want held and live", lease, found, held, live, err)
		}
		if _, found, err := tx.LoadOutboxLease(t.Context(), outboxID+99); err != nil || found {
			t.Fatalf("missing row found=%v err=%v", found, err)
		}
		return nil
	})
}

func TestLeaseLivenessComparesAsTimeAcrossWholeSecondAndFraction(t *testing.T) {
	t.Parallel()
	db, _ := openActionFixture(t)
	until := testNow.Add(500 * time.Millisecond)
	outboxID := leaseNext(t, newStore(db), "owner", until, testNow)
	cases := []struct {
		at   time.Time
		live bool
	}{
		{testNow, true}, {until.Add(-time.Nanosecond), true}, {until, false}, {until.Add(time.Second), false},
	}
	inTx(t, db, func(tx *Tx) error {
		for _, tc := range cases {
			if live, err := tx.LeaseIsLive(t.Context(), outboxID, "owner", tc.at); err != nil || live != tc.live {
				t.Errorf("live at %v = %v, err=%v, want %v", tc.at, live, err, tc.live)
			}
		}
		return nil
	})
}

var unreadableLeaseTexts = map[string]string{
	"low digit":       "1",
	"empty":           "",
	"space separated": "2999-01-01 00:00:00.000000000Z",
	"offset":          "2999-01-01T00:00:00.000000000+02:00",
	"trimmed":         "2999-01-01T00:00:00Z",
	"digits only":     "9999-99-99T99:99:99.999999999Z",
}

func TestUnreadableLeaseExpiryIsExpiredAndNeverBlocksTheQueue(t *testing.T) {
	t.Parallel()
	for name, corrupt := range unreadableLeaseTexts {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, _ := openActionFixture(t)
			execute(t, db, "UPDATE outbox SET status = 'leased', lease_owner = 'w', lease_until = ?", corrupt)
			inTx(t, db, func(tx *Tx) error {
				candidate, found, err := tx.NextCandidate(t.Context(), testNow)
				if err != nil || !found || !candidate.Lease.Unreadable || !candidate.Lease.Expired(testNow) {
					t.Fatalf("candidate=%+v found=%v err=%v, want the row selected with an unreadable, expired lease", candidate.Lease, found, err)
				}
				assertUnreadableLeaseNeverLive(t, tx, candidate)
				if ok, err := tx.AcquireLease(t.Context(), candidate.OutboxID, "next", testNow.Add(time.Minute), testNow); err != nil || !ok {
					t.Fatalf("acquire over unreadable lease ok=%v err=%v", ok, err)
				}
				return nil
			})
		})
	}
}

func assertUnreadableLeaseNeverLive(t *testing.T, tx *Tx, candidate domain.Candidate) {
	t.Helper()
	if live, err := tx.LeaseIsLive(t.Context(), candidate.OutboxID, "w", testNow); err != nil || live {
		t.Fatalf("live=%v err=%v, want never live", live, err)
	}
	if ok, err := tx.RefreshLease(t.Context(), candidate.OutboxID, "w", testNow.Add(time.Hour), testNow); err != nil || ok {
		t.Fatalf("refresh ok=%v err=%v, want refused", ok, err)
	}
	lease, found, err := tx.LoadOutboxLease(t.Context(), candidate.OutboxID)
	if held, live := lease.LeaseStanding("w", testNow); err != nil || !found || !held || live {
		t.Fatalf("standing held=%v live=%v found=%v err=%v, want held and not live", held, live, found, err)
	}
}
