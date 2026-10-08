package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
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
	ids, idArgs := storage.InClause("command_id", commandIDs)
	unresolved, statusArgs := unresolvedCommandFilter()
	return "SELECT COUNT(*) FROM commands WHERE " + ids + " AND " + unresolved, append(idArgs, statusArgs...)
}

func unresolvedCommandFilter() (string, []any) {
	return storage.InClause("status", actionport.UnresolvedCommandStatuses())
}
