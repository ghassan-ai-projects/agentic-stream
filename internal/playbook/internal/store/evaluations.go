package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const evaluationColumns = `evaluation_id, tenant_id, kind, COALESCE(fire_id, ''), COALESCE(scheduler_item_id, ''),
	COALESCE(episode_id, ''), situation_id, situation_version, in_envelope, recommendation_json, verdict`

func (t *Tx) RecordEvaluation(ctx context.Context, evaluation domain.Evaluation, now time.Time) error {
	recommendation, err := json.Marshal(evaluation.Recommendation)
	if err != nil {
		return fmt.Errorf("encode the recommendation of evaluation %s: %w", evaluation.EvaluationID, err)
	}
	if _, err := t.tx.ExecContext(ctx, `INSERT INTO rule_evaluations (evaluation_id, tenant_id, rule_id, rule_version, playbook_digest,
		kind, fire_id, scheduler_item_id, episode_id, situation_id, situation_version, in_envelope, in_cap, recommendation_json, verdict,
		created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		evaluation.EvaluationID, evaluation.TenantID, evaluation.Recommendation.RuleID, evaluation.Recommendation.RuleVersion,
		evaluation.Recommendation.PlaybookDigest, evaluation.Kind, storage.NullIfEmpty(evaluation.FireID),
		storage.NullIfEmpty(evaluation.SchedulerItemID), storage.NullIfEmpty(evaluation.EpisodeID), evaluation.SituationID,
		evaluation.SituationVersion, storage.BoolInt(evaluation.InEnvelope), storage.BoolInt(evaluation.InCap), recommendation, evaluation.Verdict,
		kernel.FormatTime(now)); err != nil {
		return fmt.Errorf("record evaluation %s: %w", evaluation.EvaluationID, err)
	}
	return nil
}

func (t *Tx) PendingAudit(ctx context.Context, schedulerItemID string) (domain.Evaluation, bool, error) {
	evaluations, err := storage.QueryAll(ctx, t.tx, "pending audit", scanEvaluation, `SELECT `+evaluationColumns+`
		FROM rule_evaluations WHERE scheduler_item_id = ? AND verdict = 'pending'`, schedulerItemID)
	if err != nil {
		return domain.Evaluation{}, false, fmt.Errorf("read the audit of scheduler item %s: %w", schedulerItemID, err)
	}
	if len(evaluations) == 0 {
		return domain.Evaluation{}, false, nil
	}
	return evaluations[0], true, nil
}

func (t *Tx) PendingEvaluations(ctx context.Context, tenantID string) ([]domain.Evaluation, error) {
	evaluations, err := storage.QueryAll(ctx, t.tx, "pending evaluations", scanEvaluation, `SELECT `+evaluationColumns+`
		FROM rule_evaluations WHERE tenant_id = ? AND verdict = 'pending' AND episode_id IS NOT NULL
		ORDER BY created_at, evaluation_id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("read pending rule evaluations: %w", err)
	}
	return evaluations, nil
}

func scanEvaluation(rows *sql.Rows) (domain.Evaluation, error) {
	var evaluation domain.Evaluation
	var inEnvelope int
	var recommendation []byte
	if err := rows.Scan(&evaluation.EvaluationID, &evaluation.TenantID, &evaluation.Kind, &evaluation.FireID,
		&evaluation.SchedulerItemID, &evaluation.EpisodeID, &evaluation.SituationID, &evaluation.SituationVersion,
		&inEnvelope, &recommendation, &evaluation.Verdict); err != nil {
		return domain.Evaluation{}, fmt.Errorf("scan rule evaluation: %w", err)
	}
	evaluation.InEnvelope = inEnvelope == 1
	if err := json.Unmarshal(recommendation, &evaluation.Recommendation); err != nil {
		return domain.Evaluation{}, fmt.Errorf("decode the recommendation of evaluation %s: %w", evaluation.EvaluationID, err)
	}
	return evaluation, nil
}

func (t *Tx) BindEvaluationEpisode(ctx context.Context, evaluationID, episodeID string) error {
	if _, err := t.tx.ExecContext(ctx, `UPDATE rule_evaluations SET episode_id = ? WHERE evaluation_id = ?`, episodeID, evaluationID); err != nil {
		return fmt.Errorf("bind evaluation %s to episode %s: %w", evaluationID, episodeID, err)
	}
	return nil
}

func (t *Tx) SettleEvaluation(ctx context.Context, evaluationID, verdict string, observed *domain.Observed, now time.Time) error {
	observedJSON, err := observedDocument(observed)
	if err != nil {
		return fmt.Errorf("encode what evaluation %s observed: %w", evaluationID, err)
	}
	if _, err := t.tx.ExecContext(ctx, `UPDATE rule_evaluations SET verdict = ?, observed_json = ?, settled_at = ?
		WHERE evaluation_id = ? AND verdict = 'pending'`, verdict, observedJSON, kernel.FormatTime(now), evaluationID); err != nil {
		return fmt.Errorf("settle evaluation %s: %w", evaluationID, err)
	}
	return nil
}

func observedDocument(observed *domain.Observed) ([]byte, error) {
	if observed == nil {
		return nil, nil
	}
	document, err := json.Marshal(observed)
	if err != nil {
		return nil, fmt.Errorf("encode the observed decision: %w", err)
	}
	return document, nil
}

func (t *Tx) CapCounts(ctx context.Context, rule domain.RuleKey, since time.Time) (domain.CapCounts, error) {
	var counts domain.CapCounts
	at := kernel.FormatTime(since)
	if err := t.tx.QueryRowContext(ctx, `SELECT
			(SELECT COUNT(*) FROM rule_evaluations WHERE rule_id = ? AND rule_version = ? AND kind = 'shadow' AND created_at >= ?) +
			(SELECT COUNT(*) FROM rule_fires WHERE rule_id = ? AND rule_version = ? AND fired_at >= ?),
			(SELECT COUNT(*) FROM rule_evaluations WHERE rule_id = ? AND rule_version = ? AND kind = 'shadow' AND in_cap = 0 AND created_at >= ?)`,
		rule.ID, rule.Version, at, rule.ID, rule.Version, at, rule.ID, rule.Version, at).Scan(&counts.Matches, &counts.OutsideCap); err != nil {
		return counts, fmt.Errorf("count the recent matches of rule %s: %w", rule.ID, err)
	}
	return counts, nil
}
