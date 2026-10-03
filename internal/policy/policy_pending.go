package policy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"time"
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
	if reason := pendingDecisionFailure(row); reason != "" {
		return nil, reason, nil
	}
	intent, reason := decodeDocument(row.IntentJSON, contractsv1.SchemaIntent)
	if reason != "" {
		return nil, reason, nil
	}
	if !canonicalDocumentMatches(row.IntentJSON, row.IntentSHA, canonicaljson.DomainIntent) {
		return nil, "intent_digest_mismatch", nil
	}
	return g.validatePendingIntent(ctx, tx, row, intent)
}

func pendingDecisionFailure(row intentRow) string {
	if row.ValidationStatus != "accepted" {
		return "decision_not_accepted"
	}
	decision, reason := decodeDocument(row.DecisionJSON, contractsv1.SchemaDecision)
	if reason != "" {
		return reason
	}
	if !matchesDecisionIdentity(row, decision) {
		return "identity_mismatch"
	}
	if !canonicalDocumentMatches(row.DecisionJSON, row.DecisionSHA, canonicaljson.DomainDecision) {
		return "decision_digest_mismatch"
	}
	return ""
}

func decodeDocument(raw []byte, schema contractsv1.SchemaName) (map[string]any, string) {
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, "schema_invalid"
	}
	if err := contractsv1.Validate(schema, document); err != nil {
		return nil, "schema_invalid"
	}
	return document, ""
}

func matchesDecisionIdentity(row intentRow, document map[string]any) bool {
	return documentString(document, "decision_id") == row.DecisionID &&
		documentString(document, "episode_id") == row.EpisodeID &&
		documentString(document, "situation_id") == row.SituationID &&
		documentInt(document, "situation_version") == row.SituationVersion &&
		row.EpisodeTenant == row.TenantID && row.SituationTenant == row.TenantID &&
		row.DecisionSituation == row.SituationID && row.DecisionVersion == row.SituationVersion &&
		row.EpisodeSituation == row.SituationID && row.EpisodeVersion == row.SituationVersion
}

func (g *Gateway) validatePendingIntent(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any) (map[string]any, string, error) {
	if reason, err := g.compensationFailure(ctx, tx, row, intent); err != nil {
		return nil, "", err
	} else if reason != "" {
		return nil, reason, nil
	}
	if !matchesIntentIdentity(row, intent) {
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

func matchesIntentIdentity(row intentRow, document map[string]any) bool {
	return documentString(document, "intent_id") == row.IntentID &&
		documentString(document, "decision_id") == row.DecisionID &&
		documentString(document, "tenant_id") == row.TenantID &&
		documentString(document, "situation_id") == row.SituationID &&
		documentInt(document, "situation_version") == row.SituationVersion &&
		documentString(document, "type") == row.IntentType &&
		documentString(document, "risk_class") == row.RiskClass
}

func (g *Gateway) evaluateFreshPending(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any, result Result, now time.Time) (Result, error) {
	if !episodeConcluded(row) {
		return g.finish(ctx, tx, row, result, "denied", "episode_not_concluded", now)
	}
	if row.CurrentSituation != row.SituationVersion {
		return g.markStale(ctx, tx, row, result, now)
	}
	return g.evaluateHealthyPending(ctx, tx, row, intent, result, now)
}

func episodeConcluded(row intentRow) bool {
	return row.EpisodeLifecycle == "concluded" || row.EpisodeLifecycle == "closed"
}

func (g *Gateway) evaluateHealthyPending(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any, result Result, now time.Time) (Result, error) {
	if sourceHealthIncomplete(row) {
		return g.finish(ctx, tx, row, result, "denied", "source_health_incomplete", now)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, row.ExpiresAt)
	if err != nil || !expiresAt.After(now) {
		return g.finish(ctx, tx, row, result, "expired", "intent_expired", now)
	}
	return g.routeIntent(ctx, tx, row, intent, result, expiresAt, now)
}

func sourceHealthIncomplete(row intentRow) bool {
	consequential := row.RiskClass == "R2" || row.RiskClass == "R3" || row.RiskClass == "R4"
	return consequential && (row.CurrentCompleteness == "provisional" || row.CurrentCompleteness == "uncertain")
}
