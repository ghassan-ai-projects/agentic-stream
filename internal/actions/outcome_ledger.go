package actions

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// unresolvedOutcomeStatuses are the command statuses whose outcome still
// awaits dispatcher reconciliation.
var unresolvedOutcomeStatuses = []string{"outcome_unknown", "reconciling", "manual_review"}

// CountUnresolvedOutcomes counts, inside tx, the commands among commandIDs
// whose outcome still awaits dispatcher reconciliation. The device authority
// uses it to refuse clearing a device while one of its commands is unresolved.
func CountUnresolvedOutcomes(ctx context.Context, tx *sql.Tx, commandIDs []string) (int64, error) {
	if len(commandIDs) == 0 {
		return 0, nil
	}
	query, args := unresolvedOutcomesQuery(commandIDs)
	var unresolved int64
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&unresolved); err != nil {
		return 0, fmt.Errorf("count unresolved command outcomes: %w", err)
	}
	return unresolved, nil
}

func unresolvedOutcomesQuery(commandIDs []string) (string, []any) {
	args := make([]any, 0, len(commandIDs)+len(unresolvedOutcomeStatuses))
	for _, commandID := range commandIDs {
		args = append(args, commandID)
	}
	for _, status := range unresolvedOutcomeStatuses {
		args = append(args, status)
	}
	return `SELECT COUNT(*) FROM commands WHERE command_id IN (` + placeholders(len(commandIDs)) +
		`) AND status IN (` + placeholders(len(unresolvedOutcomeStatuses)) + `)`, args
}

func placeholders(count int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", count), ", ")
}
