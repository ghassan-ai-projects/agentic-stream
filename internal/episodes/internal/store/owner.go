package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func ownerLostCheck(owner storage.OwnerCheck) storage.OwnerCheck {
	if owner == nil {
		return nil
	}
	return func(ctx context.Context, tx *sql.Tx, epoch string) error {
		err := owner(ctx, tx, epoch)
		if errors.Is(err, runtimecontrol.ErrRuntimeOwnerBusy) {
			return fmt.Errorf("%w: %w", episodeledger.ErrOwnerLost, err)
		}
		if err != nil {
			return fmt.Errorf("assert runtime owner: %w", err)
		}
		return nil
	}
}
