package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const caseCandidatesSQL = `
	SELECT e.tenant_id, e.situation_id, s.situation_type, s.entity_id, e.episode_id, d.decision_id, e.situation_version,
		d.raw_json, c.status, sv.snapshot_json, sv.event_horizon, COALESCE(f.rule_id, ''), COALESCE(f.rule_version, 0)
	FROM decisions d
	JOIN episodes e ON e.episode_id = d.episode_id
	JOIN intents i ON i.decision_id = d.decision_id
	JOIN commands c ON c.intent_id = i.intent_id
	JOIN situations s ON s.situation_id = e.situation_id
	JOIN situation_versions sv ON sv.situation_id = e.situation_id AND sv.version = e.situation_version
	LEFT JOIN rule_fires f ON f.episode_id = e.episode_id
	WHERE e.tenant_id = ? AND e.dispatch_policy = 'active' AND d.validation_status = 'accepted'
		AND c.effector_route != ? AND c.status IN ('succeeded', 'manual_review', 'failed', 'outcome_unknown')
		AND NOT EXISTS (SELECT 1 FROM case_verifications cv WHERE cv.situation_id = e.situation_id
			AND NOT EXISTS (SELECT 1 FROM situation_versions r WHERE r.situation_id = cv.situation_id AND r.phase = ?
				AND r.version > cv.situation_version AND r.version < e.situation_version))
	ORDER BY d.created_at, d.decision_id`

func (t *Tx) CaseCandidates(ctx context.Context, tenantID string) ([]domain.CaseCandidate, error) {
	candidates, err := storage.QueryAll(ctx, t.tx, "case candidates", scanCaseCandidate, caseCandidatesSQL, tenantID,
		actionport.WatchInstallRoute, situations.PhaseResolved)
	if err != nil {
		return nil, fmt.Errorf("read the cases to verify: %w", err)
	}
	return firstPerSituation(candidates), nil
}

func scanCaseCandidate(rows *sql.Rows) (domain.CaseCandidate, error) {
	var candidate domain.CaseCandidate
	var decision, snapshot []byte
	var status, actedAt string
	if err := rows.Scan(&candidate.TenantID, &candidate.SituationID, &candidate.SituationType, &candidate.EntityID, &candidate.EpisodeID,
		&candidate.DecisionID, &candidate.SituationVersion, &decision, &status, &snapshot, &actedAt, &candidate.RuleID,
		&candidate.RuleVersion); err != nil {
		return domain.CaseCandidate{}, fmt.Errorf("scan case candidate: %w", err)
	}
	candidate.Acknowledged = status == actionport.CommandSucceeded || status == actionport.CommandManualReview
	return decodeCaseCandidate(candidate, decision, snapshot, actedAt)
}

func decodeCaseCandidate(candidate domain.CaseCandidate, decision, snapshot []byte, actedAt string) (domain.CaseCandidate, error) {
	if err := json.Unmarshal(decision, &candidate.Decision); err != nil {
		return domain.CaseCandidate{}, fmt.Errorf("decode decision %s: %w", candidate.DecisionID, err)
	}
	var document struct {
		Facts    map[string]any `json:"facts"`
		Evidence []string       `json:"evidence"`
	}
	if err := json.Unmarshal(snapshot, &document); err != nil {
		return domain.CaseCandidate{}, fmt.Errorf("decode the snapshot of %s v%d: %w", candidate.SituationID, candidate.SituationVersion, err)
	}
	candidate.Facts, candidate.Evidence = document.Facts, document.Evidence
	return candidate, parseInto(&candidate.ActedAt, actedAt)
}

func firstPerSituation(candidates []domain.CaseCandidate) []domain.CaseCandidate {
	seen := map[string]bool{}
	first := make([]domain.CaseCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !seen[candidate.SituationID] {
			seen[candidate.SituationID] = true
			first = append(first, candidate)
		}
	}
	return first
}

func (t *Tx) RecordCase(ctx context.Context, c domain.Case, now time.Time) error {
	features, err := json.Marshal(c.Features)
	if err != nil {
		return fmt.Errorf("encode the features of case %s: %w", c.SituationID, err)
	}
	if _, err := t.tx.ExecContext(ctx, `INSERT INTO case_verifications (situation_id, tenant_id, entity_id, situation_type, episode_id,
		decision_id, situation_version, diagnosis_code, kind_digest, intent, preset, rule_id, rule_version, acted_at, window_ends_at,
		features_json, summary, verdict, reason, created_at, settled_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.SituationID, c.TenantID, c.EntityID, c.SituationType, c.EpisodeID, c.DecisionID, c.SituationVersion, c.DiagnosisCode,
		c.KindDigest, c.Intent, c.Preset, c.RuleID, c.RuleVersion, kernel.FormatTime(c.ActedAt), kernel.FormatTime(c.WindowEndsAt),
		features, c.Summary, c.Verdict, c.Reason, kernel.FormatTime(now), settledAt(c.Verdict, now)); err != nil {
		return fmt.Errorf("record case %s: %w", c.SituationID, err)
	}
	return nil
}

func settledAt(verdict string, now time.Time) sql.NullString {
	if verdict == domain.CasePending {
		return sql.NullString{}
	}
	return storage.NullIfEmpty(kernel.FormatTime(now))
}

func (t *Tx) SettleCase(ctx context.Context, c domain.Case, now time.Time) error {
	if _, err := t.tx.ExecContext(ctx, `UPDATE case_verifications SET verdict = ?, reason = ?, settled_at = ?
		WHERE situation_id = ? AND situation_version = ? AND verdict = 'pending'`, c.Verdict, c.Reason, kernel.FormatTime(now),
		c.SituationID, c.SituationVersion); err != nil {
		return fmt.Errorf("settle case %s v%d: %w", c.SituationID, c.SituationVersion, err)
	}
	return nil
}

func (t *Tx) CaseEvidence(ctx context.Context, c domain.Case) (domain.CaseEvidence, error) {
	var evidence domain.CaseEvidence
	var err error
	if evidence.ClosedAt, err = t.optionalTime(ctx, `SELECT event_horizon FROM situation_versions WHERE situation_id = ? AND phase = ?
		AND version > ? ORDER BY version LIMIT 1`, c.SituationID, situations.PhaseResolved, c.SituationVersion); err != nil {
		return evidence, err
	}
	if evidence.ReopenedAt, err = t.reopenedAfter(ctx, c, evidence.ClosedAt); err != nil {
		return evidence, err
	}
	watermark, err := t.optionalTime(ctx, `SELECT MAX(event_time) FROM event_log WHERE tenant_id = ?`, c.TenantID)
	if watermark != nil {
		evidence.Watermark = *watermark
	}
	return evidence, err
}

func (t *Tx) reopenedAfter(ctx context.Context, c domain.Case, closedAt *time.Time) (*time.Time, error) {
	if closedAt == nil {
		return nil, nil
	}
	return t.optionalTime(ctx, `SELECT MIN(sv.event_horizon) FROM situations s JOIN situation_versions sv ON sv.situation_id = s.situation_id
		WHERE s.tenant_id = ? AND s.entity_id = ? AND s.situation_type = ? AND sv.phase != ? AND sv.event_horizon > ?`,
		c.TenantID, c.EntityID, c.SituationType, situations.PhaseResolved, kernel.FormatTime(*closedAt))
}

func (t *Tx) optionalTime(ctx context.Context, query string, args ...any) (*time.Time, error) {
	var text sql.NullString
	if err := t.tx.QueryRowContext(ctx, query, args...).Scan(&text); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("read case evidence: %w", err)
	}
	if !text.Valid {
		return nil, nil
	}
	var at time.Time
	return &at, parseInto(&at, text.String)
}

func (t *Tx) PendingCases(ctx context.Context, tenantID string) ([]domain.Case, error) {
	cases, err := storage.QueryAll(ctx, t.tx, "pending cases", scanCase, `SELECT situation_id, situation_version, tenant_id, entity_id,
		situation_type, episode_id, rule_id, rule_version, acted_at, window_ends_at FROM case_verifications
		WHERE tenant_id = ? AND verdict = 'pending' ORDER BY acted_at, situation_id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("read pending cases: %w", err)
	}
	return cases, nil
}

func scanCase(rows *sql.Rows) (domain.Case, error) {
	var c domain.Case
	var actedAt, windowEndsAt string
	if err := rows.Scan(&c.SituationID, &c.SituationVersion, &c.TenantID, &c.EntityID, &c.SituationType, &c.EpisodeID, &c.RuleID,
		&c.RuleVersion, &actedAt, &windowEndsAt); err != nil {
		return domain.Case{}, fmt.Errorf("scan case: %w", err)
	}
	if err := parseInto(&c.ActedAt, actedAt); err != nil {
		return domain.Case{}, err
	}
	return c, parseInto(&c.WindowEndsAt, windowEndsAt)
}
