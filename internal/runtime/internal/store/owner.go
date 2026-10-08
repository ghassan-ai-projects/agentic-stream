package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

var errOwnerCheckMissing = errors.New("runtime owner check is not configured")

func assertRuntimeOwner(ctx context.Context, tx *sql.Tx, owner storage.OwnerCheck, epoch string) error {
	if owner == nil {
		return errOwnerCheckMissing
	}
	if err := owner(ctx, tx, epoch); err != nil {
		return fmt.Errorf("runtime ownership lost: %w", err)
	}
	return nil
}
