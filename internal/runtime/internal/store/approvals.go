package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

// ApprovalForSigning joins tenant-bound presentation to the pipeline transaction.
func (p *PipelineStore) ApprovalForSigning(ctx context.Context, r policy.ApprovalLookup) (policy.ApprovalPresentation, error) {
	return inApprovalResult(ctx, p, "present approval", r, p.Policy.ApprovalForSigning)
}

// ResolveApproval commits governance before asynchronous outbox dispatch.
func (p *PipelineStore) ResolveApproval(ctx context.Context, r policy.ApprovalResolution) (policy.Result, error) {
	return inApprovalResult(ctx, p, "resolve approval", r, p.Policy.ResolveApproval)
}

func inApprovalResult[R, T any](ctx context.Context, p *PipelineStore, operation string, request R, run func(context.Context, *sql.Tx, R) (T, error)) (T, error) {
	var result T
	err := p.inApprovalTransaction(ctx, func(tx *sql.Tx) (err error) {
		result, err = run(ctx, tx, request)
		if err != nil {
			return fmt.Errorf("%s: %w", operation, err)
		}
		return nil
	})
	return result, err
}

func (p *PipelineStore) inApprovalTransaction(ctx context.Context, run func(*sql.Tx) error) error {
	err := p.DB.WithTx(ctx, run)
	if err != nil {
		return fmt.Errorf("approval transaction: %w", err)
	}
	return nil
}
