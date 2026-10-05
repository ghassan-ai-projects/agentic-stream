package policy

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestFacadeDelegatesOwnershipFailure(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("lease lost")
	cfg := validConfig()
	cfg.OwnerEpoch = "owner"
	calls := 0
	cfg.RuntimeOwner = func(_ context.Context, tx *sql.Tx, epoch string) error {
		calls++
		if tx != nil || epoch != "owner" {
			t.Fatal("ownership inputs changed")
		}
		return sentinel
	}
	service, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r, err := service.EvaluateIntent(t.Context(), nil, EvaluationRequest{IntentID: "intent"})
	if !errors.Is(err, sentinel) || r.IntentID != "intent" {
		t.Fatal(r, err)
	}
	r, err = service.ResolveApproval(t.Context(), nil, ApprovalResolution{ID: "approval"})
	if !errors.Is(err, sentinel) || r.ApprovalID != "approval" || calls != 2 {
		t.Fatal(r, err, calls)
	}
}
