package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func policyService(t *testing.T, ownerCheck func(context.Context, *sql.Tx, string) error) *policy.Service {
	t.Helper()
	service, err := policy.New(policy.Config{PolicyVersion: "test", RuntimeOwner: ownerCheck, DecisionEpoch: func(context.Context, *sql.Tx, string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestPipelineStoreEvaluatesIntentsUnderThePolicyOwnerFence(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("owner lost")
	adapter := &PipelineStore{DB: storagetest.OpenTemp(t), Policy: policyService(t, func(context.Context, *sql.Tx, string) error { return sentinel })}
	if _, found, err := adapter.NextPendingIntent(t.Context(), "tenant"); err != nil || found {
		t.Fatalf("NextPendingIntent on an empty queue = %v, %v; want none", found, err)
	}
	err := adapter.EvaluateIntent(t.Context(), "missing", time.Now())
	if !errors.Is(err, sentinel) {
		t.Fatalf("EvaluateIntent = %v, want the owner failure", err)
	}
}

func TestPipelineStoreReportsThePolicyReadFailureUnchanged(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	adapter := &PipelineStore{DB: db, Policy: policyService(t, func(context.Context, *sql.Tx, string) error { return nil })}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	_, _, wantErr := policy.NextPendingIntent(t.Context(), db.DB, "tenant")
	if _, _, err := adapter.NextPendingIntent(t.Context(), "tenant"); err == nil || err.Error() != wantErr.Error() {
		t.Fatalf("pending read error = %v, want the original %v", err, wantErr)
	}
}
