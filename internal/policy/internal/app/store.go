package app

import (
	"context"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
	"time"
)

func (g *Service) assertOwner(ctx context.Context, tx *store.Tx) error {
	if err := tx.Assert(ctx, g.fences.RuntimeOwner, g.ownerEpoch); err != nil {
		return fmt.Errorf("policy runtime ownership lost: %w", err)
	}
	return nil
}

func (g *Service) finish(ctx context.Context, tx *store.Tx, row domain.IntentRecord, result domain.Result, policyStatus, reason string, now time.Time) (domain.Result, error) {
	return g.finishWithAuditReason(ctx, tx, row, result, policyStatus, reason, reason, now)
}

func (g *Service) finishWithAuditReason(ctx context.Context, tx *store.Tx, row domain.IntentRecord, result domain.Result, policyStatus, resultReason, auditReason string, now time.Time) (domain.Result, error) {
	if err := tx.SetIntentStatus(ctx, row.IntentID, policyStatus, now, store.SetPolicyStatus); err != nil {
		return result, err
	}
	return g.auditWithReason(ctx, tx, row, result, policyStatus, resultReason, auditReason, now)
}

func (g *Service) audit(ctx context.Context, tx *store.Tx, row domain.IntentRecord, result domain.Result, policyResult, reason string, now time.Time) (domain.Result, error) {
	return g.auditWithReason(ctx, tx, row, result, policyResult, reason, reason, now)
}

func (g *Service) auditWithReason(ctx context.Context, tx *store.Tx, row domain.IntentRecord, result domain.Result, policyResult, resultReason, auditReason string, now time.Time) (domain.Result, error) {
	result.Result, result.Reason = policyResult, resultReason
	if err := tx.RecordEvaluation(ctx, domain.EvaluationAudit{ID: g.idGen.New(ids.PrefixPolicy), PolicyVersion: g.policyVersion, PolicyDigest: g.policyDigest, Row: row, Result: result, Reason: auditReason, Now: now}); err != nil {
		return result, err
	}
	return result, nil
}
