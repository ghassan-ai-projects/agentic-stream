package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

type Store struct {
	db         *storage.DB
	signatures Signatures
}

type Tx struct {
	tx         *sql.Tx
	db         *storage.DB
	signatures Signatures
}

func New(db *storage.DB) Store {
	return Store{db: db}
}

func (s Store) WithSignatures(signatures Signatures) Store {
	s.signatures = signatures
	return s
}

func (s Store) Configured() bool { return s.db != nil }

func (s Store) Join(tx *sql.Tx) *Tx {
	return &Tx{tx: tx, db: s.db, signatures: s.signatures}
}

func (s Store) WithTx(ctx context.Context, use func(*Tx) error) error {
	if err := s.db.WithTx(ctx, func(tx *sql.Tx) error { return use(s.Join(tx)) }); err != nil {
		return fmt.Errorf("playbook transaction: %w", err)
	}
	return nil
}

type Fence = func(context.Context, *sql.Tx) error

func (s Store) WithFencedTx(ctx context.Context, fence Fence, use func(*Tx) error) error {
	return s.WithTx(ctx, func(tx *Tx) error {
		if err := fence(ctx, tx.tx); err != nil {
			return fmt.Errorf("playbook write fence: %w", err)
		}
		return use(tx)
	})
}
