package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
)

// RecordShadowDecision persists one scored shadow decision through the
// qualification-owned shadow store inside the caller's transaction.
func RecordShadowDecision(ctx context.Context, tx *sql.Tx, shadow qualification.ShadowDecision, now string) error {
	shadows := qualification.ShadowStore{}
	if err := shadows.Record(ctx, tx, shadow, now); err != nil {
		return fmt.Errorf("record shadow decision: %w", err)
	}
	return nil
}
