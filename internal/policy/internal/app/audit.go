package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

func (g *Service) assertOwner(ctx context.Context, tx *store.Tx) error {
	if err := tx.Assert(ctx, g.fences.RuntimeOwner, g.ownerEpoch); err != nil {
		return fmt.Errorf("policy runtime ownership lost: %w", err)
	}
	return nil
}
func (g *Service) finish(ctx context.Context, tx *store.Tx, e evaluation, outcome domain.Outcome) (domain.Result, error) {
	if err := tx.SetIntentStatus(ctx, domain.IntentStatusChange{IntentID: e.row.IntentID, Status: outcome.Status, Now: e.now, Operation: store.SetPolicyStatus}); err != nil {
		return e.result, err
	}
	return g.audit(ctx, tx, e, outcome)
}
func (g *Service) audit(ctx context.Context, tx *store.Tx, e evaluation, outcome domain.Outcome) (domain.Result, error) {
	e.result.Result, e.result.Reason = outcome.Status, outcome.Reason
	if err := tx.RecordEvaluation(ctx, domain.EvaluationAudit{ID: g.idGen.New(ids.PrefixPolicy), PolicyVersion: g.policyVersion, PolicyDigest: g.policyDigest, Row: e.row, Result: e.result, Reason: outcome.AuditReason(), Now: e.now}); err != nil {
		return e.result, err
	}
	return e.result, nil
}
