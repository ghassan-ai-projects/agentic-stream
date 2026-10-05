package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"testing"
	"time"
)

func TestJoinedTransactionPersistsOnlyOnCallerCommit(t *testing.T) {
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	db, id := openPolicyFixture(t, "R2", 1, 1, now.Add(time.Hour))
	defer func() { _ = db.Close() }()
	rollback := errors.New("caller rollback")
	err := db.WithTx(t.Context(), func(original *sql.Tx) error {
		tx := Join(original)
		ctx := t.Context()
		row, err := tx.LoadIntent(ctx, id)
		if err != nil {
			return err
		}
		command, err := domain.NewCommand(domain.CommandPreparation{ID: "command", PolicyDigest: "sha256:policy", Row: row, Intent: domain.ProjectIntent(map[string]any{"parameters": map[string]any{"target": "motor"}}), Now: now})
		if err != nil {
			return err
		}
		got, existing, err := tx.StoreCommandOnce(ctx, row, command, now)
		if err != nil || got.ID != command.ID || existing != "" {
			t.Fatal(got, existing, err)
		}
		got, existing, err = tx.StoreCommandOnce(ctx, row, command, now)
		if err != nil || existing != command.ID || got.ID != "" {
			t.Fatal(got, existing, err)
		}
		if err := tx.InsertCommandOutbox(ctx, command.ID, command.JSON, now); err != nil {
			return err
		}
		if err := tx.InsertCommandOutbox(ctx, command.ID, command.JSON, now); err != nil {
			return err
		}
		row.RateLimitPerHour = 1
		limited, err := tx.DispatchWithinLimit(ctx, row, now)
		if err != nil || limited {
			t.Fatal(limited, err)
		}
		limited, err = tx.DispatchWithinLimit(ctx, row, now)
		if err != nil || !limited {
			t.Fatal(limited, err)
		}
		tenant, found, err := tx.CompensationTenant(ctx, command.ID)
		if err != nil || !found || tenant != row.TenantID {
			t.Fatal(tenant, found, err)
		}
		if err := tx.RemovePreparedCommand(ctx, id, "different"); err == nil {
			t.Fatal("removed wrong command")
		}
		if err := tx.RemovePreparedCommand(ctx, id, command.ID); err != nil {
			return err
		}
		tenant, found, err = tx.CompensationTenant(ctx, command.ID)
		if err != nil || found || tenant != "" {
			t.Fatal(tenant, found, err)
		}
		if _, _, err := tx.StoreCommandOnce(ctx, row, command, now); err != nil {
			return err
		}
		if err := tx.SetIntentStatus(ctx, domain.IntentStatusChange{IntentID: id, Status: "approved", Now: now, Operation: ApproveIntentStatus}); err != nil {
			return err
		}
		if err := tx.RecordEvaluation(ctx, domain.EvaluationAudit{ID: "eval", PolicyVersion: "v1", PolicyDigest: "policy", Row: row, Result: domain.Result{Result: "approved", CommandID: command.ID}, Reason: "tested", Now: now}); err != nil {
			return err
		}
		if _, err := tx.LoadIntent(ctx, "missing"); err == nil {
			t.Fatal("missing intent accepted")
		}
		if err := tx.Assert(ctx, func(_ context.Context, got *sql.Tx, _ string) error {
			if got != original {
				t.Fatal("fence transaction changed")
			}
			return nil
		}, "owner"); err != nil {
			return err
		}
		if err := tx.AssertInterlock(ctx, interlock.DurableReader{}, row.TenantID, "motor", "R1"); err != nil {
			return err
		}
		if err := tx.AssertCalibration(ctx, func(_ context.Context, got *sql.Tx, _, _ string) error {
			if got != original {
				t.Fatal("calibration transaction changed")
			}
			return nil
		}, row.SituationType, row.ExecutorVersion); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	for _, table := range []string{"commands", "outbox", "policy_evaluations", "intent_dispatch_counts"} {
		var count int
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
}
func TestApprovalLedgerAndReadProjections(t *testing.T) {
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	db, id := openPolicyFixture(t, "R2", 1, 1, now.Add(time.Hour))
	defer func() { _ = db.Close() }()
	if err := db.WithTx(t.Context(), func(original *sql.Tx) error {
		tx := Join(original)
		ctx := t.Context()
		row, err := tx.LoadIntent(ctx, id)
		if err != nil {
			return err
		}
		if got, err := tx.PendingApproval(ctx, id); err != nil || got != "" {
			t.Fatal(got, err)
		}
		if got, err := tx.ApprovedApproval(ctx, id); err != nil || got != "" {
			t.Fatal(got, err)
		}
		if got, _, err := tx.PendingApprovalExpiry(ctx, id); err != nil || got != "" {
			t.Fatal(got, err)
		}
		sha, err := tx.ApprovalSnapshotDigest(ctx, row)
		if err != nil {
			return err
		}
		delta, err := tx.ApprovalDelta(ctx, row.EpisodeID)
		if err != nil {
			return err
		}
		intent, reason := domain.DecodeDocument(row.IntentJSON, contractsv1.SchemaIntent)
		if reason != "" {
			t.Fatal(reason)
		}
		decision, reason := domain.ParseDecision(row)
		if reason != "" {
			t.Fatal(reason)
		}
		data := domain.BuildApprovalNotification(domain.ApprovalNotice{Row: row, ID: "approval", ExpiresAt: now.Add(time.Hour), Intent: domain.ProjectIntent(intent), Context: domain.ApprovalContext{Snapshot: sha, Delta: delta, Decision: decision, Source: tx.NotificationSource(row.TenantID)}})
		request := domain.ApprovalRequest{ID: "approval", Nonce: "nonce", Data: data, JSON: []byte("{}")}
		if err := tx.RequestApproval(ctx, domain.ApprovalPublication{IntentID: id, Request: request, ExpiresAt: now.Add(time.Hour), Now: now}); err != nil {
			return err
		}
		if err := tx.AppendApprovalRequested(ctx, row, request, now); err != nil {
			return err
		}
		approval, err := tx.LoadApproval(ctx, request.ID, "tenant")
		if err != nil || approval.IntentID != id || approval.Status != "pending" {
			t.Fatal(approval, err)
		}
		got, err := tx.PendingApproval(ctx, id)
		if err != nil || got != request.ID {
			t.Fatal(got, err)
		}
		got, expiry, err := tx.PendingApprovalExpiry(ctx, id)
		if err != nil || got != request.ID || expiry != domain.FormatTime(now.Add(time.Hour)) {
			t.Fatal(got, expiry, err)
		}
		_, nonce, err := tx.AssertionBinding(ctx, request.ID)
		if err != nil || nonce != "nonce" {
			t.Fatal(nonce, err)
		}
		entity, err := tx.ApprovalEntity(ctx, row.SituationID)
		if err != nil || entity != "motor-1" {
			t.Fatal(entity, err)
		}
		active, err := tx.RelayActivity(ctx, row.TenantID, "relay-1")
		if err != nil || active != 1 {
			t.Fatal(active, err)
		}
		key, err := tx.ApproverKey(ctx, row.TenantID, "operator-1")
		if err != nil || len(key) != 32 {
			t.Fatal(key, err)
		}
		active, err = tx.ApprovalAuthority(ctx, row, entity, "operator-1")
		if err != nil || active != 1 {
			t.Fatal(active, err)
		}
		if err := tx.BindAssertion(ctx, request.ID, make([]byte, 32)); err != nil {
			return err
		}
		if err := tx.ResolveApproval(ctx, domain.ApprovalResolution{ID: request.ID, Approver: "operator-1", Relay: "relay-1", Now: now}, "approved", "resolve approval "+"approval"); err != nil {
			return err
		}
		got, err = tx.ApprovedApproval(ctx, id)
		if err != nil || got != request.ID {
			t.Fatal(got, err)
		}
		result := domain.Result{}
		row.PolicyStatus = "approval_required"
		if err := tx.BindExistingResult(ctx, row, &result); err != nil {
			return err
		}
		if err := tx.AppendApprovalResolved(ctx, domain.ApprovalEvent{Intent: row, ID: request.ID, Status: "approved", Reason: "human", Now: now}); err != nil {
			return err
		}
		request.ID = "expired"
		if err := tx.RequestApproval(ctx, domain.ApprovalPublication{IntentID: id, Request: request, ExpiresAt: now, Now: now}); err != nil {
			return err
		}
		if err := tx.ExpireApproval(ctx, request.ID, now); err != nil {
			return err
		}
		request.ID = "withdrawn"
		if err := tx.RequestApproval(ctx, domain.ApprovalPublication{IntentID: id, Request: request, ExpiresAt: now, Now: now}); err != nil {
			return err
		}
		if err := tx.WithdrawApproval(ctx, request.ID, now); err != nil {
			return err
		}
		if err := tx.AppendApprovalWithdrawn(ctx, domain.ApprovalEvent{Intent: row, ID: request.ID, Reason: "stale", Now: now}); err != nil {
			return err
		}
		request.ID = "expire-intent"
		if err := tx.RequestApproval(ctx, domain.ApprovalPublication{IntentID: id, Request: request, ExpiresAt: now, Now: now}); err != nil {
			return err
		}
		return tx.ExpireIntentApproval(ctx, id)
	}); err != nil {
		t.Fatal(err)
	}
}
func TestClosedTransactionErrorsRemainDistinguishable(t *testing.T) {
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	db, id := openPolicyFixture(t, "R2", 1, 1, now)
	defer func() { _ = db.Close() }()
	original, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := original.Rollback(); err != nil {
		t.Fatal(err)
	}
	tx := Join(original)
	row := domain.IntentRecord{IntentID: id}
	ctx := t.Context()
	checks := []func() error{
		func() error { _, err := tx.LoadIntent(ctx, id); return err }, func() error { _, err := tx.ExistingCommandID(ctx, id); return err }, func() error { _, err := tx.PendingApproval(ctx, id); return err }, func() error { _, err := tx.ApprovedApproval(ctx, id); return err }, func() error { _, _, err := tx.PendingApprovalExpiry(ctx, id); return err }, func() error { _, _, err := tx.AssertionBinding(ctx, id); return err }, func() error { _, err := tx.LoadApproval(ctx, id, "tenant"); return err }, func() error { _, err := tx.ApprovalSnapshotDigest(ctx, row); return err }, func() error { _, err := tx.ApprovalDelta(ctx, id); return err }, func() error { _, _, err := tx.CompensationTenant(ctx, id); return err }, func() error { _, err := tx.DispatchWithinLimit(ctx, row, now); return err }, func() error {
			return tx.SetIntentStatus(ctx, domain.IntentStatusChange{IntentID: id, Status: "denied", Now: now, Operation: SetPolicyStatus})
		}, func() error { return tx.RecordEvaluation(ctx, domain.EvaluationAudit{}) }, func() error { _, err := tx.InsertCommand(ctx, row, domain.CommandRecord{}, now); return err }, func() error { return tx.InsertCommandOutbox(ctx, id, nil, now) }, func() error { return tx.RemovePreparedCommand(ctx, id, id) },
	}
	for i, check := range checks {
		if err := check(); !errors.Is(err, sql.ErrTxDone) {
			t.Fatalf("check %d: %v", i, err)
		}
	}
}
