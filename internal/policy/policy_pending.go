package policy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
)

func (g *Gateway) evaluatePending(ctx context.Context, tx *sql.Tx, row intentRow, result Result, now time.Time) (Result, error) {
	intent, reason, err := g.pendingIntentDocument(ctx, tx, row)
	if err != nil {
		return result, err
	}
	if reason != "" {
		return g.finish(ctx, tx, row, result, "denied", reason, now)
	}
	return g.evaluateFreshPending(ctx, tx, row, intent, result, now)
}

func (g *Gateway) pendingIntentDocument(ctx context.Context, tx *sql.Tx, row intentRow) (map[string]any, string, error) {
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

func (g *Gateway) validatePendingIntent(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any) (map[string]any, string, error) {
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

func (g *Gateway) compensationFailure(ctx context.Context, tx *sql.Tx, row intentRow, document map[string]any) (string, error) {
	compensates, ok := document["compensates"].(string)
	if !ok || compensates == "" {
		return "", nil
	}
	var commandTenant string
	if err := tx.QueryRowContext(ctx, "SELECT tenant_id FROM commands WHERE command_id = ?", compensates).Scan(&commandTenant); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "compensation_target_missing", nil
		}
		return "", fmt.Errorf("load compensation target: %w", err)
	}
	if commandTenant != row.TenantID {
		return "compensation_tenant_mismatch", nil
	}
	return "", nil
}

func (g *Gateway) evaluateFreshPending(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any, result Result, now time.Time) (Result, error) {
	if !domain.EpisodeConcluded(row) {
		return g.finish(ctx, tx, row, result, "denied", "episode_not_concluded", now)
	}
	if row.CurrentSituation != row.SituationVersion {
		return g.markStale(ctx, tx, row, result, now)
	}
	return g.evaluateHealthyPending(ctx, tx, row, intent, result, now)
}

func (g *Gateway) evaluateHealthyPending(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any, result Result, now time.Time) (Result, error) {
	if domain.SourceHealthIncomplete(row) {
		return g.finish(ctx, tx, row, result, "denied", "source_health_incomplete", now)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, row.ExpiresAt)
	if err != nil || !expiresAt.After(now) {
		return g.finish(ctx, tx, row, result, "expired", "intent_expired", now)
	}
	return g.routeIntent(ctx, tx, row, intent, result, expiresAt, now)
}
