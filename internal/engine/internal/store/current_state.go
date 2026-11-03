package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

const currentStateSQL = `SELECT state_json FROM situations WHERE tenant_id = ? AND situation_id = ?`

func (s Store) CurrentState(ctx context.Context, situationID string) ([]byte, bool, error) {
	state, found, err := storage.QueryOptional[[]byte](ctx, s.db, currentStateSQL, s.tenantID, situationID)
	if err != nil {
		return nil, false, fmt.Errorf("read the runtime state of situation %s: %w", situationID, err)
	}
	return state, found, nil
}
