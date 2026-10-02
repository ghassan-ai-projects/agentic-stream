package control

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// NewDispatchAuthorization binds a read-only final readiness gate to one
// command's tenant and target. Adapters invoke this lower-level capability;
// they never call back into the dispatcher or another upstream service.
func NewDispatchAuthorization(db *storage.DB, reader interlock.Reader, tenantID, target string) actionport.Authorization {
	gate := dispatchGate{db: db, reader: reader, tenantID: tenantID, target: target}
	return actionport.Authorization{Check: gate.assert}
}

type dispatchGate struct {
	db               *storage.DB
	reader           interlock.Reader
	tenantID, target string
}

func (g dispatchGate) assert(ctx context.Context) error {
	if g.db == nil || g.reader == nil {
		return fmt.Errorf("dispatch readiness gate is not configured")
	}
	if err := g.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := g.reader.Assert(ctx, tx, g.tenantID, g.target, ""); err != nil {
			return fmt.Errorf("dispatch interlock assertion: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("assert dispatch interlock: %w", err)
	}
	return nil
}
