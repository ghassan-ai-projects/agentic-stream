package store

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

// ApprovalForSigning supplies the pipeline's tenant rather than trusting a caller.
func (p *PipelineStore) ApprovalForSigning(ctx context.Context, r policy.ApprovalLookup) (policy.ApprovalPresentation, error) {
	var result policy.ApprovalPresentation
	err := p.DB.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = p.Policy.ApprovalForSigning(ctx, tx, r)
		return approvalTransactionError(err)
	})
	if err != nil {
		return result, fmt.Errorf("present approval: %w", err)
	}
	return result, nil
}

// ResolveApproval commits governance before asynchronous outbox dispatch.
func (p *PipelineStore) ResolveApproval(ctx context.Context, r policy.ApprovalResolution) (policy.Result, error) {
	var result policy.Result
	err := p.DB.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = p.Policy.ResolveApproval(ctx, tx, r)
		return approvalTransactionError(err)
	})
	if err != nil {
		return result, fmt.Errorf("resolve approval: %w", err)
	}
	return result, nil
}

func approvalTransactionError(err error) error {
	if err != nil {
		return fmt.Errorf("approval transaction: %w", err)
	}
	return nil
}
