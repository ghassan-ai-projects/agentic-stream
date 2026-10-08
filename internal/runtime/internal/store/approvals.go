package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

// ApprovalForSigning joins tenant-bound presentation to the pipeline transaction.
func (p *PipelineStore) ApprovalForSigning(ctx context.Context, r policy.ApprovalLookup) (policy.ApprovalPresentation, error) {
	var result policy.ApprovalPresentation
	err := p.inApprovalTransaction(ctx, func(tx *sql.Tx) (err error) {
		result, err = p.Policy.ApprovalForSigning(ctx, tx, r)
		if err != nil {
			return fmt.Errorf("present approval: %w", err)
		}
		return nil
	})
	return result, err
}

// ResolveApproval commits governance before asynchronous outbox dispatch.
func (p *PipelineStore) ResolveApproval(ctx context.Context, r policy.ApprovalResolution) (policy.Result, error) {
	var result policy.Result
	err := p.inApprovalTransaction(ctx, func(tx *sql.Tx) (err error) {
		result, err = p.Policy.ResolveApproval(ctx, tx, r)
		if err != nil {
			return fmt.Errorf("resolve approval: %w", err)
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
