package store_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger/internal/store"
)

func TestLookupsReadPendingAndApprovedApprovalsOfAnIntent(t *testing.T) {
	t.Parallel()
	within(t, func(ctx context.Context, tx *store.Tx, _ *sql.Tx) {
		if _, found, err := tx.PendingOfIntent(ctx, "i-1"); err != nil || found {
			t.Fatalf("PendingOfIntent before any request = found %v, %v", found, err)
		}
		if err := tx.InsertPending(ctx, "a-1", "i-1", "t0", "t9", []byte("{}"), "n-1"); err != nil {
			t.Fatal(err)
		}
		pending, found, err := tx.PendingOfIntent(ctx, "i-1")
		if err != nil || !found || pending.ID != "a-1" || pending.ExpiresAt != "t9" {
			t.Fatalf("PendingOfIntent = %+v, %v, %v", pending, found, err)
		}
		binding, err := tx.PendingBinding(ctx, "a-1")
		if err != nil || binding.Nonce != "n-1" || binding.ExpiresAt != "t9" {
			t.Fatalf("PendingBinding = %+v, %v", binding, err)
		}
		if _, found, err := tx.LatestApprovedOfIntent(ctx, "i-1"); err != nil || found {
			t.Fatalf("a pending approval must not read as approved: found %v, %v", found, err)
		}
		if err := tx.DecidePending(ctx, "a-1", "approved", "human", "relay", "ok", "t1"); err != nil {
			t.Fatal(err)
		}
		approved, found, err := tx.LatestApprovedOfIntent(ctx, "i-1")
		if err != nil || !found || approved.ID != "a-1" {
			t.Fatalf("LatestApprovedOfIntent = %+v, %v, %v", approved, found, err)
		}
		if _, err := tx.PendingBinding(ctx, "a-1"); err == nil {
			t.Fatal("PendingBinding must refuse a decided approval")
		}
	})
}
