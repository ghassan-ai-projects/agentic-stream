package policy

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// Every operation asserts runtime ownership first, on the caller's transaction,
// and reports the identity it was asked about when the fence refuses.
func TestEveryOperationStopsAtAFailedOwnershipFence(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("lease lost")
	cfg := validConfig()
	cfg.OwnerEpoch = "owner"
	calls := 0
	cfg.RuntimeOwner = func(_ context.Context, tx *sql.Tx, epoch string) error {
		calls++
		if tx != nil || epoch != "owner" {
			t.Errorf("fence saw tx=%v epoch=%q, want the caller's nil tx and the configured epoch", tx, epoch)
		}
		return sentinel
	}
	service, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	evaluated, err := service.EvaluateIntent(t.Context(), nil, EvaluationRequest{IntentID: "intent"})
	if !errors.Is(err, sentinel) || evaluated.IntentID != "intent" {
		t.Fatalf("EvaluateIntent = %+v, %v", evaluated, err)
	}
	resolved, err := service.ResolveApproval(t.Context(), nil, ApprovalResolution{ID: "approval"})
	if !errors.Is(err, sentinel) || resolved.ApprovalID != "approval" {
		t.Fatalf("ResolveApproval = %+v, %v", resolved, err)
	}
	if _, err = service.ApprovalForSigning(t.Context(), nil, ApprovalLookup{ID: "approval", TenantID: "tenant"}); !errors.Is(err, sentinel) {
		t.Fatalf("ApprovalForSigning = %v", err)
	}
	if calls != 3 {
		t.Fatalf("ownership fence ran %d times, want once per operation", calls)
	}
}
