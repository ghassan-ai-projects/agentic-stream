package app

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestApprovalsReadsWhatTheLifecycleRecorded(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	if err := db.WithTx(t.Context(), func(raw *sql.Tx) error {
		tx, ctx := store.Join(raw), t.Context()
		if err := Request(ctx, tx, "apr-1", "int-1", "2026-10-08T00:00:00Z", "2026-10-08T01:00:00Z", []byte("{}"), "n-1"); err != nil {
			return err
		}
		if err := BindAssertion(ctx, tx, "apr-1", make([]byte, 32)); err != nil {
			return err
		}
		return Resolve(ctx, tx, "apr-1", "approved", "approver-1", "relay-1", "ok", "2026-10-08T00:10:00Z")
	}); err != nil {
		t.Fatal(err)
	}
	approvals, err := Approvals(t.Context(), store.NewReader(db.DB), "int-1")
	if err != nil || len(approvals) != 1 || approvals[0].Status != "approved" || approvals[0].Approver != "approver-1" || approvals[0].Relay != "relay-1" {
		t.Fatalf("approvals = %+v, %v", approvals, err)
	}
}

func TestAnApprovalReadFailureNamesTheIntent(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := Approvals(context.Background(), store.NewReader(db.DB), "int-9")
	if err == nil || !strings.HasPrefix(err.Error(), "approvals of intent int-9:") {
		t.Fatalf("Approvals on a closed database = %v, want the intent named", err)
	}
}

func TestAnIntentsApprovalIsLookedUpWhilePendingAndBoundToItsNonce(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, raw *sql.Tx) {
		seedSituation(t, ctx, raw)
		pending, found, err := PendingOfIntent(ctx, tx, "old")
		if err != nil || !found || pending.ID != "old" || pending.ExpiresAt != "later" {
			t.Fatalf("PendingOfIntent = %+v found=%v err=%v", pending, found, err)
		}
		binding, err := PendingBinding(ctx, tx, "old")
		if err != nil || binding.Nonce != "n-old" || binding.ExpiresAt != "later" {
			t.Fatalf("PendingBinding = %+v err=%v", binding, err)
		}
		must(t, Resolve(ctx, tx, "old", "approved", "human", "relay", "ok", "t1"))
		approved, found, err := LatestApprovedOfIntent(ctx, tx, "old")
		if err != nil || !found || approved.ID != "old" {
			t.Fatalf("LatestApprovedOfIntent = %+v found=%v err=%v", approved, found, err)
		}
		if _, found, _ := PendingOfIntent(ctx, tx, "old"); found {
			t.Fatal("a resolved approval is still pending")
		}
	})
}
