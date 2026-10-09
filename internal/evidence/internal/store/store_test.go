package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func openLedgerDB(t *testing.T) *storage.DB {
	t.Helper()
	db := storagetest.OpenTemp(t)

	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(t.Context(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	digest := make([]byte, 32)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO episodes (episode_id, scheduler_item_id, tenant_id, situation_id, situation_version, executor_name, executor_version, model_policy, prompt_version, snapshot_sha256, admission_key, request_json, lifecycle_status, current_attempt_id, current_fence, accepted_at) VALUES ('episode-1', 'scheduler-1', 'tenant-1', 'situation-1', 1, 'executor', 'v1', 'policy', 'prompt', ?, ?, ?, 'running', 'attempt-1', 1, '2026-08-12T12:00:00Z')`, digest, digest, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO episode_attempts (attempt_id, episode_id, fence, status, started_at) VALUES ('attempt-1', 'episode-1', 1, 'running', '2026-08-12T12:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	return db
}
func ledgerTestCall() domain.Call {
	return domain.Call{EpisodeID: "episode-1", CallID: "call-1", ToolName: "evidence.get", TenantID: "tenant-1", SituationID: "situation-1", SituationVersion: 1, EntityID: "motor-1", Arguments: domain.EvidenceGetArguments{EntityID: "motor-1"}, Deadline: time.Date(2026, 8, 12, 12, 1, 0, 0, time.UTC), AttemptID: "attempt-1", Fence: 1, Trace: traceForLedger(), MaxRows: 1, MaxBytes: 100, From: time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC), Until: time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)}
}
func traceForLedger() contractsv1.TraceContext {
	return contractsv1.TraceContext{Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"}
}
func TestStoreReadsWritesAndLeasePredicates(t *testing.T) {
	db := openLedgerDB(t)
	s := New(db, func(context.Context, *sql.Tx, string) error { return nil }, "epoch-1")
	call := ledgerTestCall()
	now := call.Until
	pending := domain.Reservation{Key: domain.ReservationKey{TenantID: call.TenantID, EpisodeID: call.EpisodeID, AttemptID: call.AttemptID, Fence: call.Fence, CallID: call.CallID}, RequestSHA256: make([]byte, 32), TokenID: "token", RuntimeEpoch: "epoch-1"}
	if !s.Configured() || s.Join(nil).Configured() {
		t.Fatal("configuration detection failed")
	}
	err := s.WithTx(t.Context(), func(tx *Tx) error {
		if err := tx.AssertOwner(t.Context()); err != nil {
			return err
		}
		state, err := tx.LiveEpisode(t.Context(), call)
		if err != nil || !state.Current {
			t.Fatalf("episode=%+v err=%v", state, err)
		}
		if _, err := tx.LiveAttempt(t.Context(), call); err != nil {
			return err
		}
		if _, err := tx.CompletionEpisode(t.Context(), pending.Key); err != nil {
			return err
		}
		if _, err := tx.CompletionAttempt(t.Context(), pending.Key); err != nil {
			return err
		}
		row, err := tx.ReadReservation(t.Context(), pending.Key)
		if err != nil || row != nil {
			t.Fatalf("new row=%+v err=%v", row, err)
		}
		if err := tx.InsertReservation(t.Context(), call, pending, now, time.Minute, "owner"); err != nil {
			return err
		}
		if err := tx.StoreResult(t.Context(), pending, domain.QueryResult{JSON: []byte(`[]`)}, now, "other"); err == nil {
			t.Fatal("foreign lease owner completed result")
		}
		if err := tx.StoreResult(t.Context(), pending, domain.QueryResult{JSON: []byte(`[]`)}, now, "owner"); err != nil {
			return err
		}
		row, err = tx.ReadReservation(t.Context(), pending.Key)
		if err != nil || row.Status != "completed" {
			t.Fatalf("row=%+v err=%v", row, err)
		}
		if err := tx.Fail(t.Context(), pending, "failed", now, "owner"); err == nil {
			t.Fatal("terminal row changed")
		}
		call.CallID = "failed"
		pending.Key.CallID = "failed"
		if err := tx.InsertReservation(t.Context(), call, pending, now, time.Minute, "owner"); err != nil {
			return err
		}
		if err := tx.Fail(t.Context(), pending, "query_failed", now, "owner"); err != nil {
			return err
		}
		call.CallID = "expired"
		pending.Key.CallID = "expired"
		if err := tx.InsertReservation(t.Context(), call, pending, now, time.Minute, "owner"); err != nil {
			return err
		}
		return tx.ReclaimExpired(t.Context(), now.Add(2*time.Minute))
	})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = recovered.Rollback() }()
	if _, err := s.Join(recovered).Recover(t.Context(), now); err != nil {
		t.Fatal(err)
	}
}
func TestStorePreservesOwnerAndDatabaseErrors(t *testing.T) {
	db := openLedgerDB(t)
	lost := errors.New("owner lost")
	s := New(db, func(context.Context, *sql.Tx, string) error { return lost }, "epoch")
	if err := s.WithTx(t.Context(), func(tx *Tx) error { return tx.AssertOwner(t.Context()) }); !errors.Is(err, lost) {
		t.Fatalf("owner=%v", err)
	}
	if err := s.WithTx(t.Context(), func(tx *Tx) error {
		_, err := tx.LiveEpisode(t.Context(), domain.Call{EpisodeID: "missing"})
		if err == nil {
			t.Fatal("unknown episode loaded")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.WithTx(t.Context(), func(*Tx) error { return nil }); err == nil {
		t.Fatal("closed db accepted")
	}
}

func TestEpisodeStateIsReadFromTheLedgerForEveryLifecycleAndBinding(t *testing.T) {
	db := openLedgerDB(t)
	s := New(db, func(context.Context, *sql.Tx, string) error { return nil }, "epoch-1")
	base := ledgerTestCall()
	lowerFence, higherFence, otherAttempt := base, base, base
	lowerFence.Fence, higherFence.Fence, otherAttempt.AttemptID = 0, 2, "attempt-2"
	calls := map[string]domain.Call{"current": base, "lower fence": lowerFence, "higher fence": higherFence, "other attempt": otherAttempt}
	lifecycles := map[episodeledger.LifecycleStatus]struct{ closed, running bool }{
		episodeledger.LifecycleAdmitted: {false, false}, episodeledger.LifecycleRunning: {false, true},
		episodeledger.LifecycleConcluded: {true, false}, episodeledger.LifecycleClosed: {true, false},
		episodeledger.LifecycleSuperseded: {true, false}, episodeledger.LifecycleExpired: {true, false},
		episodeledger.LifecycleAbandoned: {true, false},
	}
	acceptedAtReservation := map[string]bool{"admitted/current": true, "running/current": true}
	acceptedAtCompletion := map[string]bool{"running/current": true}
	for lifecycle, want := range lifecycles {
		if _, err := db.ExecContext(t.Context(), "UPDATE episodes SET lifecycle_status = ?", string(lifecycle)); err != nil {
			t.Fatal(err)
		}
		for name, call := range calls {
			err := s.WithTx(t.Context(), func(tx *Tx) error {
				key := domain.ReservationKey{TenantID: call.TenantID, EpisodeID: call.EpisodeID, AttemptID: call.AttemptID, Fence: call.Fence, CallID: call.CallID}
				wantState := domain.EpisodeState{Current: name == "current", Closed: want.closed, Running: want.running}
				live, err := tx.LiveEpisode(t.Context(), call)
				completion, completionErr := tx.CompletionEpisode(t.Context(), key)
				label := string(lifecycle) + "/" + name
				if err != nil || completionErr != nil || live != wantState || completion != wantState {
					t.Errorf("%s: live=%+v completion=%+v errors=%v %v, want %+v", label, live, completion, err, completionErr, wantState)
				}
				if accepted := domain.CheckLiveEpisode(live) == nil; accepted != acceptedAtReservation[label] {
					t.Errorf("%s: reservation accepted = %v", label, accepted)
				}
				if accepted := domain.CheckCompletionEpisode(completion) == nil; accepted != acceptedAtCompletion[label] {
					t.Errorf("%s: completion accepted = %v", label, accepted)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestEpisodeWithoutAnAttemptOrWithoutARowIsNeverCurrent(t *testing.T) {
	db := openLedgerDB(t)
	s := New(db, func(context.Context, *sql.Tx, string) error { return nil }, "epoch-1")
	call := ledgerTestCall()
	call.AttemptID, call.Fence = "", 0
	if _, err := db.ExecContext(t.Context(), "UPDATE episodes SET current_attempt_id = NULL, current_fence = 0"); err != nil {
		t.Fatal(err)
	}
	err := s.WithTx(t.Context(), func(tx *Tx) error {
		live, err := tx.LiveEpisode(t.Context(), call)
		if err != nil || live.Current {
			t.Errorf("episode without an attempt: live=%+v err=%v", live, err)
		}
		missing := domain.ReservationKey{TenantID: call.TenantID, EpisodeID: "episode-missing", AttemptID: "attempt-1", Fence: 1}
		completion, err := tx.CompletionEpisode(t.Context(), missing)
		if err != nil || completion.Current || completion.Running || domain.CheckCompletionEpisode(completion) == nil {
			t.Errorf("unknown episode at completion: %+v err=%v", completion, err)
		}
		if _, err := tx.LiveEpisode(t.Context(), domain.Call{EpisodeID: "episode-missing", TenantID: call.TenantID}); err == nil {
			t.Error("unknown episode loaded at reservation")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAttemptIsInFlightOnlyWhileDispatchedOrRunning(t *testing.T) {
	db := openLedgerDB(t)
	s := New(db, func(context.Context, *sql.Tx, string) error { return nil }, "epoch-1")
	call := ledgerTestCall()
	key := domain.ReservationKey{TenantID: call.TenantID, EpisodeID: call.EpisodeID, AttemptID: call.AttemptID, Fence: call.Fence, CallID: call.CallID}
	for attempt, want := range map[episodeledger.AttemptStatus]bool{
		episodeledger.AttemptDispatched: true, episodeledger.AttemptRunning: true, episodeledger.AttemptCancelling: false,
		episodeledger.AttemptProduced: false, episodeledger.AttemptDeclined: false, episodeledger.AttemptCancelled: false,
		episodeledger.AttemptFailed: false, episodeledger.AttemptTimedOut: false, episodeledger.AttemptAbandoned: false,
	} {
		if _, err := db.ExecContext(t.Context(), "UPDATE episode_attempts SET status = ?", string(attempt)); err != nil {
			t.Fatal(err)
		}
		err := s.WithTx(t.Context(), func(tx *Tx) error {
			live, err := tx.LiveAttempt(t.Context(), call)
			completion, completionErr := tx.CompletionAttempt(t.Context(), key)
			if err != nil || completionErr != nil || live != want || completion != want {
				t.Errorf("attempt %s: live=%v completion=%v errors=%v %v, want %v", attempt, live, completion, err, completionErr, want)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestAnAttemptThatIsNotTheFencedOneIsRefusedWhenRead(t *testing.T) {
	db := openLedgerDB(t)
	s := New(db, func(context.Context, *sql.Tx, string) error { return nil }, "epoch-1")
	for name, change := range map[string]func(*domain.Call){
		"unknown attempt": func(c *domain.Call) { c.AttemptID = "attempt-9" },
		"lower fence":     func(c *domain.Call) { c.Fence = 0 },
		"higher fence":    func(c *domain.Call) { c.Fence = 2 },
		"unknown episode": func(c *domain.Call) { c.EpisodeID = "episode-9" },
	} {
		call := ledgerTestCall()
		change(&call)
		key := domain.ReservationKey{TenantID: call.TenantID, EpisodeID: call.EpisodeID, AttemptID: call.AttemptID, Fence: call.Fence, CallID: call.CallID}
		err := s.WithTx(t.Context(), func(tx *Tx) error {
			if inFlight, err := tx.LiveAttempt(t.Context(), call); err == nil || inFlight {
				t.Errorf("%s: LiveAttempt = %v, %v", name, inFlight, err)
			}
			if inFlight, err := tx.CompletionAttempt(t.Context(), key); err == nil || inFlight {
				t.Errorf("%s: CompletionAttempt = %v, %v", name, inFlight, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestUnreadableLeaseTextIsNeverOwnedAndIsReclaimed(t *testing.T) {
	for name, corrupt := range map[string]string{
		"garbage":         "later",
		"offset":          "2999-01-01T00:00:00.000000000+02:00",
		"space separated": "2999-01-01 00:00:00.000000000Z",
		"digits only":     "9999-99-99T99:99:99.999999999Z",
	} {
		t.Run(name, func(t *testing.T) {
			db := openLedgerDB(t)
			s := New(db, func(context.Context, *sql.Tx, string) error { return nil }, "epoch-1")
			call := ledgerTestCall()
			now := call.Until
			pending := domain.Reservation{Key: domain.ReservationKey{TenantID: call.TenantID, EpisodeID: call.EpisodeID, AttemptID: call.AttemptID, Fence: call.Fence, CallID: call.CallID}, RequestSHA256: make([]byte, 32), TokenID: "token", RuntimeEpoch: "epoch-1"}
			err := s.WithTx(t.Context(), func(tx *Tx) error {
				if err := tx.InsertReservation(t.Context(), call, pending, now, time.Minute, "owner"); err != nil {
					return err
				}
				if _, err := tx.tx.ExecContext(t.Context(), "UPDATE evidence_call_ledger SET lease_until = ?", corrupt); err != nil {
					return err
				}
				if err := tx.StoreResult(t.Context(), pending, domain.QueryResult{JSON: []byte(`[]`)}, now, "owner"); err == nil {
					t.Fatal("an unreadable lease completed a result")
				}
				if err := tx.Fail(t.Context(), pending, "query_failed", now, "owner"); err == nil {
					t.Fatal("an unreadable lease recorded a failure")
				}
				return tx.ReclaimExpired(t.Context(), now)
			})
			if err != nil {
				t.Fatal(err)
			}
			var status, code string
			if err := db.QueryRowContext(t.Context(), "SELECT status, error_code FROM evidence_call_ledger").Scan(&status, &code); err != nil || status != "interrupted" || code != "lease_expired" {
				t.Fatalf("status=%q code=%q err=%v, want interrupted by lease_expired", status, code, err)
			}
		})
	}
}
