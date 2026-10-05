package runtime

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestApprovalTransactionsPreserveMissingRequestAndOwnerErrors(t *testing.T) {
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "approval.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ownerErr := errors.New("owner lost")
	for _, failure := range []error{nil, ownerErr} {
		service, err := policy.New(policy.Config{PolicyVersion: "test", Interlock: interlock.DurableReader{}, RuntimeOwner: func(context.Context, *sql.Tx, string) error { return failure }, DecisionEpoch: unownedPolicyCheck})
		if err != nil {
			t.Fatal(err)
		}
		pipeline := &Pipeline{db: db, policy: service, tenantID: "tenant", clk: clock.NewVirtual(time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))}
		expected := failure
		if expected == nil {
			expected = policy.ErrApprovalNotFound
		}
		_, err = pipeline.ApprovalForSigning(t.Context(), policy.ApprovalLookup{ID: "absent", TenantID: "foreign"})
		if !errors.Is(err, expected) {
			t.Fatal(err)
		}
		_, err = pipeline.ResolveApproval(t.Context(), policy.ApprovalResolution{ID: "absent", TenantID: "foreign"})
		if !errors.Is(err, expected) {
			t.Fatal(err)
		}
	}
}
