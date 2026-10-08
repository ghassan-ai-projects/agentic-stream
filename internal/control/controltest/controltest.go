// Package controltest is test support for the control module: it writes one
// cost limit or kill switch directly, which production configures only
// through control.ApplyCostCeilings. Import it from tests only.
package controltest

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
)

// SetCostLimit writes one ceiling or kill switch for scopeKey ("global" or
// "tenant:<id>") in the caller's transaction.
func SetCostLimit(ctx context.Context, tx *sql.Tx, scopeKey, tenantID string, maxMicro uint64, killSwitch bool, now string) error {
	if err := app.SetLimit(ctx, store.Join(tx), scopeKey, tenantID, maxMicro, killSwitch, now); err != nil {
		return fmt.Errorf("set cost limit %s: %w", scopeKey, err)
	}
	return nil
}
