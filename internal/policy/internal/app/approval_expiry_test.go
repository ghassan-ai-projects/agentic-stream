package app_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

func TestExistingApprovalExpiryAndRollback(t *testing.T) {
	for _, failure := range []string{"", "approval expiry", "notification", "intent status"} {
		t.Run(failure, func(t *testing.T) {
			f := openApprovalHTTP(t)
			service := newTestService(t)
			if failure != "" {
				query := map[string]string{
					"approval expiry": "CREATE TRIGGER fail_expiry BEFORE UPDATE ON approvals BEGIN SELECT RAISE(ABORT,'expiry failed'); END",
					"notification":    "CREATE TRIGGER fail_notice BEFORE INSERT ON notifications BEGIN SELECT RAISE(ABORT,'notice failed'); END",
					"intent status":   "CREATE TRIGGER fail_intent BEFORE UPDATE ON intents BEGIN SELECT RAISE(ABORT,'intent failed'); END",
				}[failure]
				if _, err := f.db.ExecContext(t.Context(), query); err != nil {
					t.Fatal(err)
				}
			}
			var result policy.Result
			err := f.db.WithTx(t.Context(), func(tx *sql.Tx) error {
				var err error
				result, err = service.EvaluateIntent(t.Context(), tx, policy.EvaluationRequest{IntentID: "int-policy", Now: f.clock.Now().Add(2 * time.Hour)})
				return err
			})
			if failure == "" {
				if err != nil || result.Result != "expired" || approvalStatus(t, f) != "expired" {
					t.Fatal(result, err)
				}
			} else {
				if err == nil || approvalStatus(t, f) != "pending" {
					t.Fatal(result, err)
				}
			}
			commands, outbox := approvalCounts(t, f.db)
			if commands != 0 || outbox != 0 {
				t.Fatal(commands, outbox)
			}
		})
	}
}

func TestExistingPendingApprovalDoesNotCreateAnotherRequest(t *testing.T) {
	f := openApprovalHTTP(t)
	service := newTestService(t)
	var result policy.Result
	err := f.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		var err error
		result, err = service.EvaluateIntent(t.Context(), tx, policy.EvaluationRequest{IntentID: "int-policy", Now: f.clock.Now()})
		return err
	})
	if err != nil || result.ApprovalID != f.id || result.Result != "approval_required" {
		t.Fatal(result, err)
	}
	var count int
	if err := f.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM approvals").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal(count)
	}
}
