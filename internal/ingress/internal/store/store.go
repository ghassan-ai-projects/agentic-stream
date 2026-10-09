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

// Store keeps the database private. It never exposes a raw handle.
type Store struct{ db *storage.DB }

// New binds the database without opening a transaction.
func New(db *storage.DB) Store { return Store{db: db} }

// Configured reports whether the database was supplied.
func (s Store) Configured() bool { return s.db != nil }

// LoadLine reads the last line a connector has read. A connector with no
// checkpoint starts at line 0.
func (s Store) LoadLine(ctx context.Context, connectorID string) (int, error) {
	blob, found, err := storage.QueryOptional[[]byte](ctx, s.db, "SELECT checkpoint_blob FROM connector_checkpoints WHERE connector_id = ?", connectorID)
	if err != nil {
		return 0, fmt.Errorf("load connector checkpoint: %w", err)
	}
	if !found {
		return 0, nil
	}
	return domain.DecodeCheckpoint(blob) //nolint:wrapcheck // The domain codec names the failed decode.
}

const upsertCheckpointSQL = `
		INSERT INTO connector_checkpoints (connector_id, connector_kind, checkpoint_version, checkpoint_blob, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(connector_id)
		DO UPDATE SET checkpoint_version = excluded.checkpoint_version,
		              checkpoint_blob = excluded.checkpoint_blob,
		              updated_at = excluded.updated_at`

// SaveLine records that a connector of the given kind has read lastLine lines.
func (s Store) SaveLine(ctx context.Context, connectorID, kind string, lastLine int, now time.Time) error {
	blob, err := domain.EncodeCheckpoint(lastLine)
	if err != nil {
		return err //nolint:wrapcheck // The domain codec names the failed encode.
	}
	if _, err := s.db.ExecContext(ctx, upsertCheckpointSQL, connectorID, kind, domain.CheckpointVersion, blob, kernel.FormatTime(now)); err != nil {
		return fmt.Errorf("upsert checkpoint: %w", err)
	}
	return nil
}
