package store_test

import (
	"database/sql"
	"slices"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestEveryApprovalRequestOfAnIntentIsReadOldestFirstWithHowItEnded(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	if err := db.WithTx(t.Context(), func(raw *sql.Tx) error {
		tx, ctx := store.Join(raw), t.Context()
		steps := []struct {
			id, intent, requestedAt string
			end                     func() error
		}{
			{"apr-b", "int-1", "2026-10-08T00:00:00Z", func() error {
				return tx.ExpirePending(ctx, "apr-b", "2026-10-08T01:00:00Z", domain.ReasonExpired)
			}},
			{"apr-a", "int-1", "2026-10-08T00:00:00Z", func() error {
				return tx.DecidePending(ctx, "apr-a", "approved", "approver-1", "relay-1", "looks right", "2026-10-08T00:10:00Z")
			}},
			{"apr-c", "int-1", "2026-10-08T02:00:00Z", func() error {
				return tx.WithdrawPending(ctx, "apr-c", "2026-10-08T03:00:00Z", domain.WithdrawalConflict, domain.ReasonWithdrawn)
			}},
			{"apr-other", "int-2", "2026-10-08T00:00:00Z", func() error { return nil }},
		}
		for _, step := range steps {
			if err := tx.InsertPending(ctx, step.id, step.intent, step.requestedAt, "2026-10-08T09:00:00Z", []byte("{}"), "n-"+step.id); err != nil {
				return err
			}
			if err := step.end(); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	approvals, err := store.NewReader(db.DB).Approvals(t.Context(), "int-1")
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.ApprovalView{
		{ApprovalID: "apr-a", Status: "approved", RequestedAt: "2026-10-08T00:00:00Z", ExpiresAt: "2026-10-08T09:00:00Z", DecidedAt: "2026-10-08T00:10:00Z", Approver: "approver-1", Relay: "relay-1", Reason: "looks right"},
		{ApprovalID: "apr-b", Status: "expired", RequestedAt: "2026-10-08T00:00:00Z", ExpiresAt: "2026-10-08T09:00:00Z", DecidedAt: "2026-10-08T01:00:00Z", Reason: "approval_expired"},
		{ApprovalID: "apr-c", Status: "denied", RequestedAt: "2026-10-08T02:00:00Z", ExpiresAt: "2026-10-08T09:00:00Z", DecidedAt: "2026-10-08T03:00:00Z", Reason: "approval_withdrawn", WithdrawnAt: "2026-10-08T03:00:00Z", WithdrawalReason: "situation_version_conflict"},
	}
	if !slices.Equal(approvals, want) {
		t.Fatalf("approvals =\n%+v\nwant\n%+v", approvals, want)
	}
}

func TestAnIntentWithoutApprovalRequestsHasNoApprovalViews(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	if approvals, err := store.NewReader(db.DB).Approvals(t.Context(), "int-none"); err != nil || len(approvals) != 0 {
		t.Fatalf("approvals = %+v err=%v, want none", approvals, err)
	}
}

func TestReadingApprovalsFromAClosedDatabaseNamesTheRead(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.NewReader(db.DB).Approvals(t.Context(), "int-1"); err == nil || !strings.HasPrefix(err.Error(), "read approvals:") {
		t.Fatalf("Approvals on a closed database = %v, want it wrapped as read approvals", err)
	}
}
