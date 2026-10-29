package runtime

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook"
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

// PresentRuleProposal presents a pending playbook rule proposal and the bytes
// its approver signs.
func (p *Pipeline) PresentRuleProposal(ctx context.Context, r playbook.ProposalLookup) (playbook.ProposalPresentation, error) {
	return p.useCases().PresentRuleProposal(ctx, r)
}

// ResolveRuleProposal commits a signed approval or rejection of a playbook
// rule proposal; an approval writes the rule, in shadow, to the playbook file.
func (p *Pipeline) ResolveRuleProposal(ctx context.Context, r playbook.ProposalResolution) (playbook.ProposalResult, error) {
	return p.useCases().ResolveRuleProposal(ctx, r)
}
