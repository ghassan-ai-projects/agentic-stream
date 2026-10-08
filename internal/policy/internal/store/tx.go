// Package store owns policy SQL and joins the caller's governance transaction.
package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// OwnerCheck is a write fence run on the policy transaction.
type OwnerCheck = storage.OwnerCheck

type Tx struct{ tx *sql.Tx }

func Join(tx *sql.Tx) *Tx { return &Tx{tx: tx} }

func (tx *Tx) Assert(ctx context.Context, fence storage.OwnerCheck, epoch string) error {
	return fence(ctx, tx.tx, epoch)
}

func (tx *Tx) AssertInterlock(ctx context.Context) error {
	if err := interlock.Assert(ctx, tx.tx); err != nil {
		return fmt.Errorf("assert action interlock: %w", err)
	}
	return nil
}
