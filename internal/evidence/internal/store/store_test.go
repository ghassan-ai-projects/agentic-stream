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

func TestEpisodeAndAttemptStateFollowTheLedgerPredicates(t *testing.T) {
	db := openLedgerDB(t)
	s := New(db, func(context.Context, *sql.Tx, string) error { return nil }, "epoch-1")
	call := ledgerTestCall()
	for _, lifecycle := range []episodeledger.LifecycleStatus{episodeledger.LifecycleAdmitted, episodeledger.LifecycleRunning, episodeledger.LifecycleConcluded, episodeledger.LifecycleClosed, episodeledger.LifecycleSuperseded, episodeledger.LifecycleExpired, episodeledger.LifecycleAbandoned} {
		if _, err := db.ExecContext(t.Context(), "UPDATE episodes SET lifecycle_status = ?", string(lifecycle)); err != nil {
			t.Fatal(err)
		}
		err := s.WithTx(t.Context(), func(tx *Tx) error {
			live, err := tx.LiveEpisode(t.Context(), call)
			if err != nil || live.Closed != lifecycle.Closed() || live.Running != (lifecycle == episodeledger.LifecycleRunning) {
				t.Errorf("lifecycle %s: live=%+v err=%v", lifecycle, live, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, attempt := range []episodeledger.AttemptStatus{episodeledger.AttemptDispatched, episodeledger.AttemptRunning, episodeledger.AttemptCancelling, episodeledger.AttemptProduced, episodeledger.AttemptDeclined, episodeledger.AttemptCancelled, episodeledger.AttemptFailed, episodeledger.AttemptTimedOut, episodeledger.AttemptAbandoned} {
		if _, err := db.ExecContext(t.Context(), "UPDATE episode_attempts SET status = ?", string(attempt)); err != nil {
			t.Fatal(err)
		}
		err := s.WithTx(t.Context(), func(tx *Tx) error {
			inFlight, err := tx.LiveAttempt(t.Context(), call)
			if err != nil || inFlight != attempt.InFlight() {
				t.Errorf("attempt %s: inFlight=%v err=%v", attempt, inFlight, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestEvidenceFenceVerdictsMatchTheLedgerIdentityRules(t *testing.T) {
	db := openLedgerDB(t)
	s := New(db, func(context.Context, *sql.Tx, string) error { return nil }, "epoch-1")
	base := ledgerTestCall()
	stale, wrong, otherTenant := base, base, base
	stale.Fence, wrong.AttemptID, otherTenant.TenantID = 0, "attempt-2", "tenant-2"
	for _, lifecycle := range []episodeledger.LifecycleStatus{episodeledger.LifecycleAdmitted, episodeledger.LifecycleRunning, episodeledger.LifecycleConcluded, episodeledger.LifecycleSuperseded} {
		if _, err := db.ExecContext(t.Context(), "UPDATE episodes SET lifecycle_status = ?", string(lifecycle)); err != nil {
			t.Fatal(err)
		}
		for name, call := range map[string]domain.Call{"current": base, "stale fence": stale, "wrong attempt": wrong} {
			err := s.WithTx(t.Context(), func(tx *Tx) error {
				state, err := tx.LiveEpisode(t.Context(), call)
				if err != nil {
					return err
				}
				fence, _, err := episodeledger.ReadEpisodeFence(t.Context(), tx.tx, call.EpisodeID)
				if err != nil {
					return err
				}
				identity := episodeledger.Identity{EpisodeID: call.EpisodeID, AttemptID: call.AttemptID, Fence: call.Fence}
				if refused := fence.CheckOpenIdentity(identity) != nil; refused != (domain.CheckLiveEpisode(state) != nil) {
					t.Errorf("%s/%s: ledger refused=%v evidence=%v", lifecycle, name, refused, domain.CheckLiveEpisode(state))
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	err := s.WithTx(t.Context(), func(tx *Tx) error {
		if _, err := tx.LiveEpisode(t.Context(), otherTenant); err == nil {
			t.Error("episode of another tenant loaded")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
