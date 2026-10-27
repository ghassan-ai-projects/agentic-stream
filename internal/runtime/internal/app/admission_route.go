package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/rule"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
)

func (a *Admitter) routeThroughPlaybook(ctx context.Context, tx *store.AdmissionTx, req *episodes.Request) error {
	route, err := tx.Route(ctx, req)
	if err != nil {
		return err
	}
	return applyRoute(req, route, tx.PlaybookDigest())
}

func applyRoute(req *episodes.Request, route playbook.Route, playbookDigest string) error {
	switch route.Kind {
	case playbook.RouteRule:
		if err := req.BindExecutor(rule.ExecutorName, playbookDigest, rule.Section, route.Recommendation); err != nil {
			return fmt.Errorf("bind episode %s to its rule: %w", req.EpisodeID, err)
		}
	case playbook.RouteAudit:
		req.DispatchInShadow()
	}
	return nil
}
