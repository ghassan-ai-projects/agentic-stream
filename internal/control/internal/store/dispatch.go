package store

import (
	"context"
	"database/sql"
	"fmt"
)

// AssertInterlock asks the governance interlock whether the tenant's target may
// receive a command, in one read transaction.
func (s Store) AssertInterlock(ctx context.Context, reader Interlock, tenantID, target string) error {
	if err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := reader.Assert(ctx, tx, tenantID, target, ""); err != nil {
			return fmt.Errorf("dispatch interlock assertion: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("assert dispatch interlock: %w", err)
	}
	return nil
}
