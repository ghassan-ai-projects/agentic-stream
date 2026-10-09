package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestApprovalOperationsRunInsideOneFencedTransaction(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("owner lost")
	adapter := &PipelineStore{DB: storagetest.OpenTemp(t), Policy: policyService(t, func(context.Context, *sql.Tx, string) error { return sentinel })}
	if _, err := adapter.ApprovalForSigning(t.Context(), policy.ApprovalLookup{ID: "missing"}); !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "present approval") {
		t.Fatalf("ApprovalForSigning = %v, want the owner failure naming the operation", err)
	}
	if _, err := adapter.ResolveApproval(t.Context(), policy.ApprovalResolution{ID: "missing"}); !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "resolve approval") {
		t.Fatalf("ResolveApproval = %v, want the owner failure naming the operation", err)
	}
}

func TestApprovalOperationsReportATransactionThatCannotBegin(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	adapter := &PipelineStore{DB: db, Policy: policyService(t, func(context.Context, *sql.Tx, string) error { return nil })}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ResolveApproval(t.Context(), policy.ApprovalResolution{ID: "any"}); err == nil || !strings.Contains(err.Error(), "approval transaction") {
		t.Fatalf("ResolveApproval on a closed database = %v, want an approval transaction failure", err)
	}
}
