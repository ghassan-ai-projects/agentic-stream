package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
)

// CountUnresolvedOutcomes counts, in this transaction, the commands among
// commandIDs whose outcome still awaits reconciliation.
func (tx *Tx) CountUnresolvedOutcomes(ctx context.Context, commandIDs []string) (int64, error) {
	if len(commandIDs) == 0 {
		return 0, nil
	}
	query, args := unresolvedOutcomesQuery(commandIDs)
	var unresolved int64
	if err := tx.tx.QueryRowContext(ctx, query, args...).Scan(&unresolved); err != nil {
		return 0, fmt.Errorf("count unresolved command outcomes: %w", err)
	}
	return unresolved, nil
}

func unresolvedOutcomesQuery(commandIDs []string) (string, []any) {
	statuses := domain.UnresolvedCommandStatuses
	args := make([]any, 0, len(commandIDs)+len(statuses))
	for _, commandID := range commandIDs {
		args = append(args, commandID)
	}
	for _, status := range statuses {
		args = append(args, status)
	}
	return `SELECT COUNT(*) FROM commands WHERE command_id IN (` + placeholders(len(commandIDs)) +
		`) AND status IN (` + placeholders(len(statuses)) + `)`, args
}

func placeholders(count int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", count), ", ")
}
