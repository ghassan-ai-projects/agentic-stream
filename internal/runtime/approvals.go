package runtime

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

// ApprovalForSigning supplies the pipeline's tenant rather than trusting a caller.
func (p *Pipeline) ApprovalForSigning(ctx context.Context, r policy.ApprovalLookup) (policy.ApprovalPresentation, error) {
	r.TenantID = p.tenantID
	var result policy.ApprovalPresentation
	err := p.db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = p.policy.ApprovalForSigning(ctx, tx, r)
		return approvalTransactionError(err)
	})
	if err != nil {
		return result, fmt.Errorf("present approval: %w", err)
	}
	return result, nil
}

// ResolveApproval commits governance before asynchronous outbox dispatch.
func (p *Pipeline) ResolveApproval(ctx context.Context, r policy.ApprovalResolution) (policy.Result, error) {
	r.TenantID, r.Now = p.tenantID, p.clk.Now().UTC()
	var result policy.Result
	err := p.db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = p.policy.ResolveApproval(ctx, tx, r)
		return approvalTransactionError(err)
	})
	if err != nil {
		return result, fmt.Errorf("resolve approval: %w", err)
	}
	return result, nil
}

func (p *Pipeline) maintainApprovedWork(ctx context.Context) error {
	if err := p.watch.Expire(ctx); err != nil {
		return fmt.Errorf("expire watches: %w", err)
	}
	report := PipelineReport{}
	if err := p.dispatchApprovedCommands(ctx, &report); err != nil {
		return err
	}
	p.observeBatch(report)
	return nil
}

func approvalTransactionError(err error) error {
	if err != nil {
		return fmt.Errorf("approval transaction: %w", err)
	}
	return nil
}
