package app

import (
	"context"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
	"time"
)

func (g *Service) evaluatePending(ctx context.Context, tx *store.Tx, row domain.IntentRecord, result domain.Result, now time.Time) (domain.Result, error) {
	intent, reason, err := g.pendingIntentDocument(ctx, tx, row)
	if err != nil {
		return result, err
	}
	if reason != "" {
		return g.finish(ctx, tx, row, result, "denied", reason, now)
	}
	return g.evaluateFreshPending(ctx, tx, row, intent, result, now)
}

func (g *Service) pendingIntentDocument(ctx context.Context, tx *store.Tx, row domain.IntentRecord) (map[string]any, string, error) {
	if reason := domain.PendingDecisionFailure(row); reason != "" {
		return nil, reason, nil
	}
	intent, reason := domain.DecodeDocument(row.IntentJSON, contractsv1.SchemaIntent)
	if reason != "" {
		return nil, reason, nil
	}
	if !domain.CanonicalDocumentMatches(row.IntentJSON, row.IntentSHA, canonicaljson.DomainIntent) {
		return nil, "intent_digest_mismatch", nil
	}
	return g.validatePendingIntent(ctx, tx, row, intent)
}

func (g *Service) validatePendingIntent(ctx context.Context, tx *store.Tx, row domain.IntentRecord, intent map[string]any) (map[string]any, string, error) {
	if reason, err := g.compensationFailure(ctx, tx, row, intent); err != nil {
		return nil, "", err
	} else if reason != "" {
		return nil, reason, nil
	}
	if !domain.MatchesIntentIdentity(row, intent) {
		return nil, "identity_mismatch", nil
	}
	return intent, "", nil
}

func (g *Service) compensationFailure(ctx context.Context, tx *store.Tx, row domain.IntentRecord, document map[string]any) (string, error) {
	compensates, ok := document["compensates"].(string)
	if !ok || compensates == "" {
		return "", nil
	}
	commandTenant, found, err := tx.CompensationTenant(ctx, compensates)
	if err != nil {
		return "", err
	}
	if !found {
		return "compensation_target_missing", nil
	}
	if commandTenant != row.TenantID {
		return "compensation_tenant_mismatch", nil
	}
	return "", nil
}

func (g *Service) evaluateFreshPending(ctx context.Context, tx *store.Tx, row domain.IntentRecord, intent map[string]any, result domain.Result, now time.Time) (domain.Result, error) {
	if !domain.EpisodeConcluded(row) {
		return g.finish(ctx, tx, row, result, "denied", "episode_not_concluded", now)
	}
	if row.CurrentSituation != row.SituationVersion {
		return g.markStale(ctx, tx, row, result, now)
	}
	return g.evaluateHealthyPending(ctx, tx, row, intent, result, now)
}

func (g *Service) evaluateHealthyPending(ctx context.Context, tx *store.Tx, row domain.IntentRecord, intent map[string]any, result domain.Result, now time.Time) (domain.Result, error) {
	if domain.SourceHealthIncomplete(row) {
		return g.finish(ctx, tx, row, result, "denied", "source_health_incomplete", now)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, row.ExpiresAt)
	if err != nil || !expiresAt.After(now) {
		return g.finish(ctx, tx, row, result, "expired", "intent_expired", now)
	}
	return g.routeIntent(ctx, tx, row, intent, result, expiresAt, now)
}
