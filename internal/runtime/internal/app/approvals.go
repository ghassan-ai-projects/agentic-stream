package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

// ApprovalForSigning presents immutable evidence in the pipeline tenant.
func (p *Pipeline) ApprovalForSigning(ctx context.Context, r policy.ApprovalLookup) (policy.ApprovalPresentation, error) {
	r.TenantID = p.tenantID
	return p.transactions.ApprovalForSigning(ctx, r)
}

// ResolveApproval uses the trusted clock before committing governance.
func (p *Pipeline) ResolveApproval(ctx context.Context, r policy.ApprovalResolution) (policy.Result, error) {
	r.TenantID, r.Now = p.tenantID, p.clk.Now().UTC()
	return p.transactions.ResolveApproval(ctx, r)
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
