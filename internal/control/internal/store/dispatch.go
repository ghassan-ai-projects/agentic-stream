package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
)

// AssertInterlock asks the governance interlock whether the tenant's target may
// receive a command, in one read transaction.
func (s Store) AssertInterlock(ctx context.Context) error {
	if err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := interlock.Assert(ctx, tx); err != nil {
			return fmt.Errorf("dispatch interlock assertion: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("assert dispatch interlock: %w", err)
	}
	return nil
}
