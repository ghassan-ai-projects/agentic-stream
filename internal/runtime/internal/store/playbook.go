package store

import (
	"context"
	"database/sql"
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

func (p *PipelineStore) SettlePlaybook(ctx context.Context) error {
	if p.Playbook == nil {
		return nil
	}
	if err := p.Playbook.Settle(ctx); err != nil {
		return fmt.Errorf("settle the playbook evidence: %w", err)
	}
	return nil
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
	return inRuleProposal(ctx, p, "present rule proposal", r, (*playbook.Service).PresentProposal)
}

func (p *PipelineStore) ResolveRuleProposal(ctx context.Context, r playbook.ProposalResolution) (playbook.ProposalResult, error) {
	return inRuleProposal(ctx, p, "resolve rule proposal", r, (*playbook.Service).ResolveProposal)
}

func inRuleProposal[R, T any](ctx context.Context, p *PipelineStore, operation string, request R,
	run func(*playbook.Service, context.Context, *sql.Tx, R) (T, error)) (T, error) {
	if p.Playbook == nil {
		var none T
		return none, playbook.ErrProposalNotFound
	}
	return inApprovalResult(ctx, p, operation, request, func(ctx context.Context, tx *sql.Tx, request R) (T, error) {
		return run(p.Playbook, ctx, tx, request)
	})
}
