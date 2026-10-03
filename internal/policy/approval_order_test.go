package policy

import (
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
)

func TestApprovalStalenessPrecedesExpiryAndAuthorization(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	db, intentID := openPolicyFixture(t, "R2", 1, 1, now.Add(time.Hour))
	defer func() { _ = db.Close() }()
	gateway := NewGateway("policy-v1", ids.Deterministic())
	var requested, resolved Result
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		requested, err = gateway.EvaluateIntent(ctx, tx, intentID, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if requested.ApprovalID == "" {
		t.Fatalf("request = %+v", requested)
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE situations SET current_version = 2 WHERE situation_id = 'sit-policy'"); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE approvals SET expires_at = ? WHERE approval_id = ?", formatTime(now), requested.ApprovalID); err != nil {
			return err
		}
		var err error
		resolved, err = gateway.ResolveApproval(ctx, tx, requested.ApprovalID, true, "same", "same", nil, "", now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if resolved.Result != "denied" || resolved.Reason != "situation_version_conflict" {
		t.Fatalf("resolution = %+v", resolved)
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM approvals WHERE approval_id = ?", requested.ApprovalID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "denied" {
		t.Fatalf("status = %q", status)
	}
	var commands int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM commands").Scan(&commands); err != nil {
		t.Fatal(err)
	}
	if commands != 0 {
		t.Fatalf("commands = %d", commands)
	}
}
