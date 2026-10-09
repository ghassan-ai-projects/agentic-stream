// Package store owns the connector checkpoint SQL. It selects and writes data;
// where a connector resumes is decided in domain and app.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/ingress/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type Store struct{ db *storage.DB }

func New(db *storage.DB) Store { return Store{db: db} }

func (s Store) Configured() bool { return s.db != nil }

func (s Store) LoadLine(ctx context.Context, connectorID string) (int, error) {
	blob, found, err := s.loadBlob(ctx, connectorID)
	if err != nil || !found {
		return 0, err
	}
	line, err := domain.DecodeCheckpoint(blob)
	if err != nil {
		return 0, fmt.Errorf("connector checkpoint %s: %w", connectorID, err)
	}
	return line, nil
}

func (s Store) loadBlob(ctx context.Context, connectorID string) ([]byte, bool, error) {
	blob, found, err := storage.QueryOptional[[]byte](ctx, s.db, "SELECT checkpoint_blob FROM connector_checkpoints WHERE connector_id = ?", connectorID)
	if err != nil {
		return nil, false, fmt.Errorf("load connector checkpoint %s: %w", connectorID, err)
	}
	return blob, found, nil
}

const upsertCheckpointSQL = `
		INSERT INTO connector_checkpoints (connector_id, connector_kind, checkpoint_version, checkpoint_blob, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(connector_id)
		DO UPDATE SET checkpoint_version = excluded.checkpoint_version,
		              checkpoint_blob = excluded.checkpoint_blob,
		              updated_at = excluded.updated_at`

func (s Store) SaveLine(ctx context.Context, connectorID, kind string, lastLine int, now time.Time) error {
	blob, err := domain.EncodeCheckpoint(lastLine)
	if err != nil {
		return fmt.Errorf("connector checkpoint %s: %w", connectorID, err)
	}
	return s.saveBlob(ctx, connectorID, kind, blob, now)
}

func (s Store) saveBlob(ctx context.Context, connectorID, kind string, blob []byte, now time.Time) error {
	if _, err := s.db.ExecContext(ctx, upsertCheckpointSQL, connectorID, kind, domain.CheckpointVersion, blob, kernel.FormatTime(now)); err != nil {
		return fmt.Errorf("upsert checkpoint %s: %w", connectorID, err)
	}
	return nil
}

func (s Store) LoadPosition(ctx context.Context, connectorID string) (domain.Checkpoint, error) {
	blob, found, err := s.loadBlob(ctx, connectorID)
	if err != nil || !found {
		return domain.Checkpoint{}, err
	}
	position, err := domain.DecodePosition(blob)
	if err != nil {
		return domain.Checkpoint{}, fmt.Errorf("connector checkpoint %s: %w", connectorID, err)
	}
	return position, nil
}

func (s Store) SavePosition(ctx context.Context, connectorID, kind string, position domain.Checkpoint, now time.Time) error {
	blob, err := domain.EncodePosition(position)
	if err != nil {
		return fmt.Errorf("connector checkpoint %s: %w", connectorID, err)
	}
	return s.saveBlob(ctx, connectorID, kind, blob, now)
}
