package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/approvalledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

const loadAuthorizationRecordsSQL = `
			SELECT c.tenant_id, c.intent_id, c.effector_route, c.normalized_target,
			       c.idempotency_key, c.command_json, c.command_sha256,
			       i.tenant_id, i.intent_id, i.decision_id, i.situation_id,
			       i.situation_version, i.intent_type, i.risk_class, i.intent_json,
			       i.intent_sha256, i.expires_at, i.policy_status, i.requires_approval,
			       d.validation_status, d.raw_json, d.decision_sha256, d.situation_id, d.situation_version, d.episode_id,
			       e.tenant_id, e.situation_id, e.situation_version, e.lifecycle_status,
			       s.tenant_id, s.last_material_version
			FROM commands c
			JOIN intents i ON i.intent_id = c.intent_id
			JOIN decisions d ON d.decision_id = i.decision_id
			JOIN episodes e ON e.episode_id = d.episode_id
			JOIN situations s ON s.situation_id = i.situation_id
			WHERE c.command_id = ? AND c.status = 'dispatching'`

func (tx *Tx) LoadAuthorizationRecords(ctx context.Context, commandID string) (domain.AuthorizationRecords, error) {
	r, err := tx.scanAuthorizationRecords(ctx, commandID)
	if err != nil {
		return domain.AuthorizationRecords{}, err
	}
	if r.Approval, err = tx.loadApprovedApproval(ctx, r.Intent.ID); err != nil {
		return domain.AuthorizationRecords{}, err
	}
	return r, nil
}

func (tx *Tx) scanAuthorizationRecords(ctx context.Context, commandID string) (domain.AuthorizationRecords, error) {
	var r domain.AuthorizationRecords
	var lifecycle episodeledger.LifecycleStatus
	var intentExpiry string
	dests := concat(commandDests(&r.Command), intentDests(&r.Intent, &intentExpiry),
		decisionDests(&r.Decision), episodeDests(&r.Episode, &lifecycle), []any{&r.Situation.TenantID, &r.Situation.LastMaterialVersion})
	if err := tx.tx.QueryRowContext(ctx, loadAuthorizationRecordsSQL, commandID).Scan(dests...); err != nil {
		return domain.AuthorizationRecords{}, fmt.Errorf("load authorization records: %w", err)
	}
	r.Command.ID = commandID
	r.Episode.ProducedDecision = lifecycle.ProducedDecision()
	var err error
	if r.Intent.ExpiresAt, err = kernel.ParseTime(intentExpiry); err != nil {
		return domain.AuthorizationRecords{}, fmt.Errorf("parse intent expiry of command %s: %w", commandID, err)
	}
	return r, nil
}

func (tx *Tx) loadApprovedApproval(ctx context.Context, intentID string) (domain.ApprovalRow, error) {
	approval, present, err := approvalledger.LatestApprovedOfIntent(ctx, tx.tx, intentID)
	if err != nil {
		return domain.ApprovalRow{}, fmt.Errorf("load approved approval of intent %s: %w", intentID, err)
	}
	if !present {
		return domain.ApprovalRow{}, nil
	}
	expiresAt, err := kernel.ParseTime(approval.ExpiresAt)
	if err != nil {
		return domain.ApprovalRow{}, fmt.Errorf("parse approval %s expiry: %w", approval.ID, err)
	}
	return domain.ApprovalRow{ID: approval.ID, ExpiresAt: expiresAt, Present: true}, nil
}

func (tx *Tx) ApprovedPolicyDigest(ctx context.Context, intentID string) (string, error) {
	var digest string
	if err := tx.tx.QueryRowContext(ctx, `
		SELECT policy_digest FROM policy_evaluations
		WHERE intent_id = ? AND result = 'approved'
		ORDER BY evaluated_at DESC LIMIT 1`, intentID).Scan(&digest); err != nil {
		return "", fmt.Errorf("load approved policy digest: %w", err)
	}
	return digest, nil
}

func commandDests(c *domain.CommandRow) []any {
	return []any{&c.TenantID, &c.IntentID, &c.Route, &c.Target, &c.Idempotency, &c.JSON, &c.SHA}
}

func intentDests(i *domain.IntentRow, expiresAt *string) []any {
	return []any{&i.TenantID, &i.ID, &i.DecisionID, &i.SituationID, &i.Version, &i.Type, &i.Risk, &i.JSON, &i.SHA, expiresAt, &i.PolicyStatus, &i.RequiresApproval}
}

func decisionDests(d *domain.DecisionRow) []any {
	return []any{&d.ValidationStatus, &d.JSON, &d.SHA, &d.SituationID, &d.SituationVersion, &d.EpisodeID}
}

func episodeDests(e *domain.EpisodeRow, lifecycle *episodeledger.LifecycleStatus) []any {
	return []any{&e.TenantID, &e.SituationID, &e.SituationVersion, lifecycle}
}

func concat(groups ...[]any) []any {
	var all []any
	for _, group := range groups {
		all = append(all, group...)
	}
	return all
}
