// Package store owns policy SQL and joins the caller's governance transaction.
package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
)

// Fence runs a lower control check on the original transaction.
type Fence func(context.Context, *sql.Tx, string) error

// CalibrationCheck verifies the exact executor calibration artifact.
type CalibrationCheck func(context.Context, *sql.Tx, string, string) error

// Tx is a caller-owned transaction; it never begins or commits a transaction.
type Tx struct{ tx *sql.Tx }

// Join wraps the original transaction without changing its ownership.
func Join(tx *sql.Tx) *Tx { return &Tx{tx: tx} }

// Assert invokes a required ownership or epoch check.
func (tx *Tx) Assert(ctx context.Context, fence Fence, epoch string) error {
	return fence(ctx, tx.tx, epoch)
}

// AssertInterlock asks the action-readiness reader using the same transaction.
func (tx *Tx) AssertInterlock(ctx context.Context, reader interlock.Reader, tenant, target, risk string) error {
	if err := reader.Assert(ctx, tx.tx, tenant, target, risk); err != nil {
		return fmt.Errorf("assert action interlock: %w", err)
	}
	return nil
}

// AssertCalibration asks the configured permission source on the same transaction.
func (tx *Tx) AssertCalibration(ctx context.Context, check CalibrationCheck, situationType, executorVersion string) error {
	return check(ctx, tx.tx, situationType, executorVersion)
}
