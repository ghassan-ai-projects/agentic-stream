package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook"
)

func (t *AdmissionTx) Route(ctx context.Context, req *episodes.Request) (playbook.Route, error) {
	if t.store.Playbook == nil {
		return playbook.Route{Kind: playbook.RouteModel}, nil
	}
	admission, err := admissionOf(t.store.TenantID, req)
	if err != nil {
		return playbook.Route{}, err
	}
	route, err := t.store.Playbook.Route(ctx, t.tx, admission)
	if err != nil {
		return playbook.Route{}, fmt.Errorf("route episode %s through the playbook: %w", req.EpisodeID, err)
	}
	return route, nil
}

func admissionOf(tenantID string, req *episodes.Request) (playbook.Admission, error) {
	facts, evidence, err := req.SnapshotFacts()
	if err != nil {
		return playbook.Admission{}, fmt.Errorf("read the snapshot to route episode %s: %w", req.EpisodeID, err)
	}
	eventTime, err := req.EvidenceHorizon()
	if err != nil {
		return playbook.Admission{}, fmt.Errorf("read the event time to route episode %s: %w", req.EpisodeID, err)
	}
	return playbook.Admission{TenantID: tenantID, EpisodeID: req.EpisodeID, SchedulerItemID: req.SchedulerItemID, SituationID: req.SituationID,
		SituationVersion: req.SituationVersion, EventTime: eventTime, Facts: facts, Evidence: evidence}, nil
}

func (t *AdmissionTx) PlaybookDigest() string {
	if t.store.Playbook == nil {
		return ""
	}
	return t.store.Playbook.Digest()
}

func (p *PipelineStore) SettlePlaybook(ctx context.Context) (playbook.Report, error) {
	if p.Playbook == nil {
		return playbook.Report{}, nil
	}
	report, err := p.Playbook.Settle(ctx)
	if err != nil {
		return report, fmt.Errorf("settle the playbook evidence: %w", err)
	}
	return report, nil
}

func (p *PipelineStore) DraftPlaybook(ctx context.Context) error {
	if p.Playbook == nil {
		return nil
	}
	if _, err := p.Playbook.Draft(ctx); err != nil {
		return fmt.Errorf("draft a playbook rule: %w", err)
	}
	if _, err := p.Playbook.Promote(ctx); err != nil {
		return fmt.Errorf("promote playbook rules: %w", err)
	}
	return nil
}

func (p *PipelineStore) PresentRuleProposal(ctx context.Context, r playbook.ProposalLookup) (playbook.ProposalPresentation, error) {
	if p.Playbook == nil {
		return playbook.ProposalPresentation{}, playbook.ErrProposalNotFound
	}
	return inApprovalResult(ctx, p, "present rule proposal", r, p.Playbook.PresentProposal)
}

func (p *PipelineStore) ResolveRuleProposal(ctx context.Context, r playbook.ProposalResolution) (playbook.ProposalResult, error) {
	if p.Playbook == nil {
		return playbook.ProposalResult{}, playbook.ErrProposalNotFound
	}
	return inApprovalResult(ctx, p, "resolve rule proposal", r, p.Playbook.ResolveProposal)
}
