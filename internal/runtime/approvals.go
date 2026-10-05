package runtime

import (
	"context"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

// ApprovalForSigning presents a tenant-scoped approval through the pipeline.
func (p *Pipeline) ApprovalForSigning(ctx context.Context, r policy.ApprovalLookup) (policy.ApprovalPresentation, error) {
	return p.useCases().ApprovalForSigning(ctx, r)
}

// ResolveApproval commits a human decision using runtime tenant and clock.
func (p *Pipeline) ResolveApproval(ctx context.Context, r policy.ApprovalResolution) (policy.Result, error) {
	return p.useCases().ResolveApproval(ctx, r)
}
