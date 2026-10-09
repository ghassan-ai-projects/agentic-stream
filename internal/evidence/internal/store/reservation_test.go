package store

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
)

func TestReservationIsAbsentUntilInsertedThenReadsBackAsRunning(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	call := ledgerCall("call-1")
	pending := pendingReservation(call)
	mustInTx(t, s, func(tx *Tx) error {
		row, err := tx.ReadReservation(t.Context(), pending.Key)
		if err != nil || row != nil {
			t.Fatalf("row before insert = %+v, err %v", row, err)
		}
		if err := tx.InsertReservation(t.Context(), call, pending, callUntil, time.Minute, "owner"); err != nil {
			return err
		}
		row, err = tx.ReadReservation(t.Context(), pending.Key)
		if err != nil || row.Status != "running" || row.TokenID != "token" || row.Epoch != "epoch-1" || len(row.RequestHash) != 32 || row.ResultJSON != nil {
			t.Fatalf("row after insert = %+v, err %v", row, err)
		}
		return nil
	})
}

func TestReservationReadIsScopedToTheFullCallIdentity(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	pending := reserve(t, s, "call-1", "owner")
	tests := map[string]func(*domain.ReservationKey){
		"tenant":  func(k *domain.ReservationKey) { k.TenantID = "tenant-2" },
		"episode": func(k *domain.ReservationKey) { k.EpisodeID = "episode-2" },
		"attempt": func(k *domain.ReservationKey) { k.AttemptID = "attempt-2" },
		"fence":   func(k *domain.ReservationKey) { k.Fence = 2 },
		"call":    func(k *domain.ReservationKey) { k.CallID = "call-2" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			key := pending.Key
			mutate(&key)
			mustInTx(t, s, func(tx *Tx) error {
				if row, err := tx.ReadReservation(t.Context(), key); err != nil || row != nil {
					t.Fatalf("row for another %s = %+v, err %v", name, row, err)
				}
				return nil
			})
		})
	}
}

func TestInsertReservationRefusesARepeatedCallIdentity(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	reserve(t, s, "call-1", "owner")
	call := ledgerCall("call-1")
	err := inTx(t, s, func(tx *Tx) error {
		return tx.InsertReservation(t.Context(), call, pendingReservation(call), callUntil, time.Minute, "owner")
	})
	if err == nil || !strings.Contains(err.Error(), "reserve evidence call") {
		t.Fatalf("second insert error = %v, want a reserve failure", err)
	}
}

func TestStoreResultCompletesOnlyUnderTheReservingLease(t *testing.T) {
	t.Parallel()
	result := domain.QueryResult{JSON: []byte(`{"rows":[]}`), RowCount: 3}
	tests := []struct {
		name    string
		mutate  func(*domain.Reservation)
		owner   string
		at      time.Time
		wantErr bool
	}{
		{"reserving owner within the lease", func(*domain.Reservation) {}, "owner", callUntil, false},
		{"another lease owner", func(*domain.Reservation) {}, "other", callUntil, true},
		{"another token", func(r *domain.Reservation) { r.TokenID = "other" }, "owner", callUntil, true},
		{"another runtime epoch", func(r *domain.Reservation) { r.RuntimeEpoch = "epoch-2" }, "owner", callUntil, true},
		{"another request", func(r *domain.Reservation) { r.RequestSHA256 = append([]byte{1}, make([]byte, 31)...) }, "owner", callUntil, true},
		{"lease already expired", func(*domain.Reservation) {}, "owner", callUntil.Add(time.Minute), true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s, db := openStore(t)
			pending := reserve(t, s, "call-1", "owner")
			test.mutate(&pending)
			err := inTx(t, s, func(tx *Tx) error {
				return tx.StoreResult(t.Context(), pending, result, test.at, test.owner)
			})
			status, _ := ledgerState(t, db, "call-1")
			if test.wantErr {
				if err == nil || !strings.Contains(err.Error(), "no longer owned") || status != "running" {
					t.Fatalf("error = %v, status %q, want a refused completion leaving the call running", err, status)
				}
				return
			}
			if err != nil || status != "completed" {
				t.Fatalf("error = %v, status %q, want a completed call", err, status)
			}
		})
	}
}

func TestStoredResultReadsBackWithItsExactBytesDigestAndCounts(t *testing.T) {
	t.Parallel()
	s, _ := openStore(t)
	pending := reserve(t, s, "call-1", "owner")
	result := domain.QueryResult{JSON: []byte(`{"rows":[{"v":1}]}`), RowCount: 1}
	mustInTx(t, s, func(tx *Tx) error {
		if err := tx.StoreResult(t.Context(), pending, result, callUntil, "owner"); err != nil {
			return err
		}
		row, err := tx.ReadReservation(t.Context(), pending.Key)
		if err != nil || row.Status != "completed" || string(row.ResultJSON) != string(result.JSON) ||
			string(row.ResultHash) != string(result.SHA256()) || row.ResultBytes != int64(len(result.JSON)) || row.RowCount != 1 {
			t.Fatalf("row = %+v, err %v", row, err)
		}
		return nil
	})
}

func TestTerminalCallsAcceptNoFurtherResultOrFailure(t *testing.T) {
	t.Parallel()
	s, db := openStore(t)
	completed := reserve(t, s, "completed", "owner")
	failed := reserve(t, s, "failed", "owner")
	mustInTx(t, s, func(tx *Tx) error {
		if err := tx.StoreResult(t.Context(), completed, domain.QueryResult{JSON: []byte(`[]`)}, callUntil, "owner"); err != nil {
			return err
		}
		return tx.Fail(t.Context(), failed, "query_failed", callUntil, "owner")
	})
	err := inTx(t, s, func(tx *Tx) error {
		if err := tx.Fail(t.Context(), completed, "late", callUntil, "owner"); err == nil {
			t.Error("a completed call was failed")
		}
		if err := tx.StoreResult(t.Context(), failed, domain.QueryResult{JSON: []byte(`[]`)}, callUntil, "owner"); err == nil {
			t.Error("a failed call was completed")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if status, code := ledgerState(t, db, "completed"); status != "completed" || code != "" {
		t.Fatalf("completed call = %q/%q", status, code)
	}
	if status, code := ledgerState(t, db, "failed"); status != "failed" || code != "query_failed" {
		t.Fatalf("failed call = %q/%q", status, code)
	}
}

func TestFailRecordsTheStableCodeOnlyForTheReservingLease(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		owner   string
		at      time.Time
		wantErr bool
	}{
		{"reserving owner within the lease", "owner", callUntil, false},
		{"another lease owner", "other", callUntil, true},
		{"lease already expired", "owner", callUntil.Add(time.Minute), true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s, db := openStore(t)
			pending := reserve(t, s, "call-1", "owner")
			err := inTx(t, s, func(tx *Tx) error {
				return tx.Fail(t.Context(), pending, "result_bytes_exceeded", test.at, test.owner)
			})
			status, code := ledgerState(t, db, "call-1")
			if test.wantErr {
				if err == nil || status != "running" {
					t.Fatalf("error = %v, status %q, want a refused failure leaving the call running", err, status)
				}
				return
			}
			if err != nil || status != "failed" || code != "result_bytes_exceeded" {
				t.Fatalf("error = %v, call %q/%q, want failed/result_bytes_exceeded", err, status, code)
			}
		})
	}
}

func TestReclaimExpiredInterruptsOnlyExpiredAndForeignEpochRunningCalls(t *testing.T) {
	t.Parallel()
	s, db := openStore(t)
	reserve(t, s, "live", "owner")
	expired := ledgerCall("expired")
	foreign := ledgerCall("foreign")
	foreignPending := pendingReservation(foreign)
	foreignPending.RuntimeEpoch = "epoch-old"
	completed := reserve(t, s, "completed", "owner")
	mustInTx(t, s, func(tx *Tx) error {
		if err := tx.InsertReservation(t.Context(), expired, pendingReservation(expired), callUntil.Add(-time.Hour), time.Minute, "owner"); err != nil {
			return err
		}
		if err := tx.InsertReservation(t.Context(), foreign, foreignPending, callUntil, time.Minute, "owner"); err != nil {
			return err
		}
		return tx.StoreResult(t.Context(), completed, domain.QueryResult{JSON: []byte(`[]`)}, callUntil, "owner")
	})

	mustInTx(t, s, func(tx *Tx) error { return tx.ReclaimExpired(t.Context(), callUntil.Add(30*time.Second)) })

	want := map[string][2]string{
		"live": {"running", ""}, "expired": {"interrupted", "lease_expired"},
		"foreign": {"interrupted", "lease_expired"}, "completed": {"completed", ""},
	}
	for callID, expected := range want {
		if status, code := ledgerState(t, db, callID); status != expected[0] || code != expected[1] {
			t.Errorf("call %s = %q/%q, want %q/%q", callID, status, code, expected[0], expected[1])
		}
	}
}

func TestRecoverInterruptsOnlyRunningCallsOfAPriorEpoch(t *testing.T) {
	t.Parallel()
	s, db := openStore(t)
	reserve(t, s, "current", "owner")
	old := ledgerCall("old")
	oldPending := pendingReservation(old)
	oldPending.RuntimeEpoch = "epoch-old"
	oldCompleted := ledgerCall("old-completed")
	oldCompletedPending := pendingReservation(oldCompleted)
	oldCompletedPending.RuntimeEpoch = "epoch-old"
	mustInTx(t, s, func(tx *Tx) error {
		if err := tx.InsertReservation(t.Context(), old, oldPending, callUntil, time.Minute, "owner"); err != nil {
			return err
		}
		if err := tx.InsertReservation(t.Context(), oldCompleted, oldCompletedPending, callUntil, time.Minute, "owner"); err != nil {
			return err
		}
		return tx.StoreResult(t.Context(), oldCompletedPending, domain.QueryResult{JSON: []byte(`[]`)}, callUntil, "owner")
	})

	var recovered int
	mustInTx(t, s, func(tx *Tx) error {
		var err error
		recovered, err = tx.Recover(t.Context(), callUntil)
		return err
	})

	if recovered != 1 {
		t.Fatalf("recovered = %d, want 1", recovered)
	}
	want := map[string][2]string{"current": {"running", ""}, "old": {"interrupted", "runtime_restart"}, "old-completed": {"completed", ""}}
	for callID, expected := range want {
		if status, code := ledgerState(t, db, callID); status != expected[0] || code != expected[1] {
			t.Errorf("call %s = %q/%q, want %q/%q", callID, status, code, expected[0], expected[1])
		}
	}
	mustInTx(t, s, func(tx *Tx) error {
		if again, err := tx.Recover(t.Context(), callUntil); err != nil || again != 0 {
			t.Fatalf("repeat recovery = %d, err %v, want zero", again, err)
		}
		return nil
	})
}

func TestUnreadableLeaseTextIsNeverOwnedAndIsReclaimed(t *testing.T) {
	t.Parallel()
	for name, corrupt := range map[string]string{
		"garbage":         "later",
		"offset":          "2999-01-01T00:00:00.000000000+02:00",
		"space separated": "2999-01-01 00:00:00.000000000Z",
		"digits only":     "9999-99-99T99:99:99.999999999Z",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, db := openStore(t)
			pending := reserve(t, s, "call-1", "owner")
			if _, err := db.ExecContext(t.Context(), "UPDATE evidence_call_ledger SET lease_until = ?", corrupt); err != nil {
				t.Fatal(err)
			}
			mustInTx(t, s, func(tx *Tx) error {
				if err := tx.StoreResult(t.Context(), pending, domain.QueryResult{JSON: []byte(`[]`)}, callUntil, "owner"); err == nil {
					t.Error("an unreadable lease completed a result")
				}
				if err := tx.Fail(t.Context(), pending, "query_failed", callUntil, "owner"); err == nil {
					t.Error("an unreadable lease recorded a failure")
				}
				return tx.ReclaimExpired(t.Context(), callUntil)
			})
			if status, code := ledgerState(t, db, "call-1"); status != "interrupted" || code != "lease_expired" {
				t.Fatalf("call = %q/%q, want interrupted/lease_expired", status, code)
			}
		})
	}
}
