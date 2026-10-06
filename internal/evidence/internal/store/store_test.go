package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func openLedgerDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(t.Context(), filepath.Join(dir, "runtime.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
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
		if err != nil || state.AttemptID != call.AttemptID {
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
