package app

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestApprovalsBindTheRuntimeTenantAndSurfaceTheOriginalFailure(t *testing.T) {
	t.Parallel()
	ownerLost := errors.New("owner lost")
	tests := []struct {
		name    string
		failure error
		want    error
	}{
		{"an absent request is not found, whatever tenant the caller named", nil, policy.ErrApprovalNotFound},
		{"a lost owner refuses before anything is read", ownerLost, ownerLost},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			service, err := policy.New(policy.Config{PolicyVersion: "test", RuntimeOwner: func(context.Context, *sql.Tx, string) error { return tt.failure }, DecisionEpoch: func(context.Context, *sql.Tx, string) error { return nil }})
			if err != nil {
				t.Fatal(err)
			}
			pipeline := &Pipeline{transactions: &store.PipelineStore{DB: storagetest.OpenTemp(t), Policy: service}, tenantID: "tenant", clk: sources.NewVirtual(time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))}
			if _, err := pipeline.ApprovalForSigning(t.Context(), policy.ApprovalLookup{ID: "absent", TenantID: "foreign"}); !errors.Is(err, tt.want) {
				t.Fatalf("ApprovalForSigning = %v, want %v", err, tt.want)
			}
			if _, err := pipeline.ResolveApproval(t.Context(), policy.ApprovalResolution{ID: "absent", TenantID: "foreign"}); !errors.Is(err, tt.want) {
				t.Fatalf("ResolveApproval = %v, want %v", err, tt.want)
			}
		})
	}
}
