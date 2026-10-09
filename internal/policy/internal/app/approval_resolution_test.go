package app_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

func TestADeniedApprovalIsFinalAndNeverCommands(t *testing.T) {
	t.Parallel()
	p := openPendingApproval(t)

	denied, err := p.resolve(t, p.signedResolution(t, false))
	if err != nil || denied.Result != "denied" || denied.Reason != "approval_denied" || denied.CommandID != "" {
		t.Fatalf("denial = %+v, %v; want denied/approval_denied without a command", denied, err)
	}
	if status := p.status(t); status != "denied" {
		t.Fatalf("approval status = %q, want denied", status)
	}
	if commands, outbox := commandAndOutboxCounts(t, p.db); commands != 0 || outbox != 0 {
		t.Fatalf("commands=%d outbox=%d after a denial", commands, outbox)
	}
}

func TestResolvingAnApprovalAgainReportsItAlreadyResolved(t *testing.T) {
	t.Parallel()
	p := openPendingApproval(t)
	resolution := p.signedResolution(t, true)
	if _, err := p.resolve(t, resolution); err != nil {
		t.Fatal(err)
	}

	again, err := p.resolve(t, resolution)
	if err != nil || again.Reason != "approval_already_resolved" || again.CommandID != "" {
		t.Fatalf("second resolution = %+v, %v; want approval_already_resolved without a second command", again, err)
	}
	if commands, outbox := commandAndOutboxCounts(t, p.db); commands != 1 || outbox != 1 {
		t.Fatalf("commands=%d outbox=%d after a repeated resolution, want 1/1", commands, outbox)
	}
}

// Invariant 7 at the human gate: an approval is a decision about the world as
// it was presented. Resolving it re-checks the intent against current state,
// and a changed basis produces no command even though a human said yes.
func TestResolvingAnApprovalRevalidatesTheIntentAgainstCurrentState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		change        string
		wait          time.Duration
		wantResult    string
		wantReason    string
		wantStatus    string
		wantWithdrawn bool
	}{
		{name: "the Situation moved on to a newer material version", change: newerMaterialVersion, wantResult: "denied", wantReason: "situation_version_conflict", wantStatus: "denied", wantWithdrawn: true},
		{name: "the approval expired before the human answered", wait: 2 * time.Hour, wantResult: "expired", wantReason: "approval_expired", wantStatus: "expired"},
		{name: "the action interlock tripped", change: tripInterlock, wantResult: "denied", wantReason: "interlock_not_ready", wantStatus: "approved"},
		{name: "source health became incomplete", change: uncertainSourceHealth, wantResult: "denied", wantReason: "source_health_incomplete", wantStatus: "approved"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			p := openPendingApproval(t)
			resolution := p.signedResolution(t, true)
			if test.change != "" {
				exec(t, p.db, test.change)
			}
			resolution.Now = p.now.Add(test.wait)

			result, err := p.resolve(t, resolution)
			if err != nil || result.Result != test.wantResult || result.Reason != test.wantReason {
				t.Fatalf("result = %+v, %v; want %s/%s", result, err, test.wantResult, test.wantReason)
			}
			commands, outbox := commandAndOutboxCounts(t, p.db)
			if status := p.status(t); status != test.wantStatus || commands != 0 || outbox != 0 {
				t.Fatalf("approval status=%q commands=%d outbox=%d, want %s with nothing dispatched", status, commands, outbox, test.wantStatus)
			}
			if withdrawn := scalar[bool](t, p.db, "SELECT withdrawn_at IS NOT NULL FROM approvals WHERE approval_id = ?", p.approvalID); withdrawn != test.wantWithdrawn {
				t.Fatalf("approval withdrawn = %t, want %t", withdrawn, test.wantWithdrawn)
			}
		})
	}
}

func TestApprovalStalenessPrecedesExpiryAndAuthorization(t *testing.T) {
	t.Parallel()
	p := openPendingApproval(t)
	var resolved policy.Result
	inTx(t, p.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(t.Context(), "UPDATE situations SET current_version = 2, last_material_version = 2 WHERE situation_id = 'sit-policy'"); err != nil {
			return err
		}
		if _, err := tx.ExecContext(t.Context(), "UPDATE approvals SET expires_at = ? WHERE approval_id = ?", kernel.FormatTime(p.now), p.approvalID); err != nil {
			return err
		}
		var err error
		resolved, err = p.service.ResolveApproval(t.Context(), tx, policy.ApprovalResolution{TenantID: "tenant", ID: p.approvalID, Approved: true, Approver: "same", Relay: "same", Now: p.now})
		return err
	})

	if resolved.Result != "denied" || resolved.Reason != "situation_version_conflict" {
		t.Fatalf("resolution = %+v, want denied/situation_version_conflict before expiry or authorization is considered", resolved)
	}
	if commands, _ := commandAndOutboxCounts(t, p.db); commands != 0 {
		t.Fatalf("commands = %d, want 0", commands)
	}
}
