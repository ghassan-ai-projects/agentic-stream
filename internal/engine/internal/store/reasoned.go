package store

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

const reasonedVersionSQL = "SELECT phase, version, event_horizon FROM situation_versions WHERE situation_id = ? AND version = ?"

func (s Store) ReasonedVersion(ctx context.Context, situationID string, version int) (domain.ReasonedVersion, error) {
	var reasoned domain.ReasonedVersion
	var horizon string
	if err := s.db.QueryRowContext(ctx, reasonedVersionSQL, situationID, version).Scan(&reasoned.Phase, &reasoned.Version, &horizon); err != nil {
		return reasoned, fmt.Errorf("read situation %s version %d: %w", situationID, version, err)
	}
	at, err := kernel.ParseTime(horizon)
	if err != nil {
		return reasoned, fmt.Errorf("parse the event horizon of situation %s version %d: %w", situationID, version, err)
	}
	reasoned.Horizon = at
	return reasoned, nil
}
