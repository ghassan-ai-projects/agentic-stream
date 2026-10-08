// Package store owns policy SQL and joins the caller's governance transaction.
package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
)

type Fence func(context.Context, *sql.Tx, string) error

type Tx struct{ tx *sql.Tx }

func Join(tx *sql.Tx) *Tx { return &Tx{tx: tx} }

func (tx *Tx) Assert(ctx context.Context, fence Fence, epoch string) error {
	return fence(ctx, tx.tx, epoch)
}

func (tx *Tx) AssertInterlock(ctx context.Context) error {
	if err := interlock.Assert(ctx, tx.tx); err != nil {
		return fmt.Errorf("assert action interlock: %w", err)
	}
	return nil
}
