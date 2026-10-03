package cognition

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// RecordCostRejectionReason appends the cost-control refusal to its trigger evaluation.
func RecordCostRejectionReason(ctx context.Context, tx *sql.Tx, schedulerItemID string, rejection error) error {
	triggerID, reasons, err := loadEvaluationReasons(ctx, tx, schedulerItemID)
	if err != nil {
		return err
	}
	reasonsJSON, err := json.Marshal(append(reasons, "episode admission rejected by cost control: "+rejection.Error()))
	if err != nil {
		return fmt.Errorf("encode trigger evaluation reasons: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE trigger_evaluations SET reasons_json = ? WHERE trigger_id = ?",
		reasonsJSON, triggerID); err != nil {
		return fmt.Errorf("record cost rejection reason: %w", err)
	}
	return nil
}

// loadEvaluationReasons reads the reasons of the evaluation that admitted the
// scheduler item.
func loadEvaluationReasons(ctx context.Context, tx *sql.Tx, schedulerItemID string) (string, []string, error) {
	var triggerID string
	var reasonsJSON []byte
	if err := tx.QueryRowContext(ctx, `
			SELECT trigger_id, reasons_json FROM trigger_evaluations
			WHERE trigger_id = (SELECT trigger_id FROM scheduler_items WHERE scheduler_item_id = ?)`, schedulerItemID).
		Scan(&triggerID, &reasonsJSON); err != nil {
		return "", nil, fmt.Errorf("load cost-rejected trigger evaluation: %w", err)
	}
	var reasons []string
	if len(reasonsJSON) > 0 {
		if err := json.Unmarshal(reasonsJSON, &reasons); err != nil {
			return "", nil, fmt.Errorf("decode trigger evaluation reasons: %w", err)
		}
	}
	return triggerID, reasons, nil
}
