package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Store opens read-only snapshots of the run database.
type Store struct {
	db *storage.DB
}

// New wraps the run database.
func New(db *storage.DB) Store { return Store{db: db} }

// IsZero reports whether the store has no database.
func (s Store) IsZero() bool { return s.db == nil }

// Snapshot is one consistent read-only transaction.
type Snapshot struct {
	tx *sql.Tx
}

// InSnapshot runs fn in one read-only transaction and commits it, so every
// ledger the artifact exports reflects the same instant.
func (s Store) InSnapshot(ctx context.Context, fn func(*Snapshot) error) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin run artifact snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(&Snapshot{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit run artifact snapshot: %w", err)
	}
	return nil
}
