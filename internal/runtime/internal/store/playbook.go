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
	facts, evidence, err := req.SnapshotFacts()
	if err != nil {
		return playbook.Route{}, fmt.Errorf("read the snapshot to route episode %s: %w", req.EpisodeID, err)
	}
	route, err := t.store.Playbook.Route(ctx, t.tx, playbook.Admission{TenantID: t.store.TenantID, EpisodeID: req.EpisodeID,
		SchedulerItemID: req.SchedulerItemID, SituationID: req.SituationID, SituationVersion: req.SituationVersion,
		Facts: facts, Evidence: evidence})
	if err != nil {
		return playbook.Route{}, fmt.Errorf("route episode %s through the playbook: %w", req.EpisodeID, err)
	}
	return route, nil
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
