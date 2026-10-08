package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
)

// Reader reads trigger evaluations from a database without a transaction.
type Reader struct{ db *sql.DB }

// NewReader binds a trigger-evaluation reader to an open database.
func NewReader(db *sql.DB) Reader { return Reader{db: db} }

const triggerEvaluationColumns = `
	SELECT trigger_id, trigger_name, situation_id, situation_version, score, threshold, lane, outcome,
		reasons_json, delta_json, policy_sha256, evaluated_at
	FROM trigger_evaluations`

// TriggerEvaluation reads one of the tenant's trigger evaluations.
func (r Reader) TriggerEvaluation(ctx context.Context, tenantID, triggerID string) (domain.TriggerEvaluationRecord, error) {
	rows, err := r.db.QueryContext(ctx, triggerEvaluationColumns+` WHERE tenant_id = ? AND trigger_id = ?`, tenantID, triggerID)
	if err != nil {
		return domain.TriggerEvaluationRecord{}, fmt.Errorf("read trigger evaluation: %w", err)
	}
	records, err := collectEvaluations(rows)
	if err != nil {
		return domain.TriggerEvaluationRecord{}, err
	}
	if len(records) == 0 {
		return domain.TriggerEvaluationRecord{}, fmt.Errorf("trigger evaluation %s: %w", triggerID, sql.ErrNoRows)
	}
	return records[0], nil
}

// TriggerEvaluations reads every trigger evaluation of one Situation version.
func (r Reader) TriggerEvaluations(ctx context.Context, tenantID, situationID string, version int) ([]domain.TriggerEvaluationRecord, error) {
	rows, err := r.db.QueryContext(ctx, triggerEvaluationColumns+` WHERE tenant_id = ? AND situation_id = ? AND situation_version = ? ORDER BY trigger_name`, tenantID, situationID, version)
	if err != nil {
		return nil, fmt.Errorf("read trigger evaluations: %w", err)
	}
	return collectEvaluations(rows)
}

func collectEvaluations(rows *sql.Rows) ([]domain.TriggerEvaluationRecord, error) {
	defer func() { _ = rows.Close() }()
	var records []domain.TriggerEvaluationRecord
	for rows.Next() {
		record, err := scanEvaluation(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err() //nolint:wrapcheck // The iteration error is the driver's.
}

func scanEvaluation(rows *sql.Rows) (domain.TriggerEvaluationRecord, error) {
	var record domain.TriggerEvaluationRecord
	var reasons, delta, policy []byte
	if err := rows.Scan(&record.TriggerID, &record.TriggerName, &record.SituationID, &record.SituationVersion, &record.Score, &record.Threshold, &record.Lane, &record.Outcome, &reasons, &delta, &policy, &record.EvaluatedAt); err != nil {
		return domain.TriggerEvaluationRecord{}, fmt.Errorf("scan trigger evaluation: %w", err)
	}
	if err := json.Unmarshal(reasons, &record.Reasons); err != nil {
		return domain.TriggerEvaluationRecord{}, fmt.Errorf("decode trigger reasons %s: %w", record.TriggerID, err)
	}
	record.Delta, record.PolicySHA256 = delta, "sha256:"+hex.EncodeToString(policy)
	return record, nil
}
