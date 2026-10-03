package cognition

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// RecordCostRejectionReason appends the cost-control refusal to its trigger evaluation.
func RecordCostRejectionReason(ctx context.Context, tx *sql.Tx, schedulerItemID string, rejection error) error {
	var triggerID string
	var reasonsJSON []byte
	if err := tx.QueryRowContext(ctx, `
			SELECT trigger_id, reasons_json FROM trigger_evaluations
			WHERE trigger_id = (SELECT trigger_id FROM scheduler_items WHERE scheduler_item_id = ?)`, schedulerItemID).
		Scan(&triggerID, &reasonsJSON); err != nil {
		return fmt.Errorf("load cost-rejected trigger evaluation: %w", err)
	}
	var reasons []string
	if len(reasonsJSON) > 0 {
		if err := json.Unmarshal(reasonsJSON, &reasons); err != nil {
			return fmt.Errorf("decode trigger evaluation reasons: %w", err)
		}
	}
	reasons = append(reasons, "episode admission rejected by cost control: "+rejection.Error())
	reasonsJSON, err := json.Marshal(reasons)
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
