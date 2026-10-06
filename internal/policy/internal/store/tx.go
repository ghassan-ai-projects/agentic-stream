// Package store owns policy SQL and joins the caller's governance transaction.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
)

// Fence runs a lower control check on the original transaction.
type Fence func(context.Context, *sql.Tx, string) error

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

// CalibrationActive reports whether an ACTIVE calibration artifact matches the
// Situation type (the domain) and the executor version (the model revision,
// the compiled-spec digest that binds prompt, diagnosis catalog and policy). A
// spec change therefore invalidates the artifact. The table has no writer in
// the runtime: an operator provisions artifacts.
func (tx *Tx) CalibrationActive(ctx context.Context, situationType, executorVersion string) (bool, error) {
	var active int
	err := tx.tx.QueryRowContext(ctx, `
		SELECT active FROM calibration_artifacts
		WHERE domain = ? AND model_revision = ? AND active = 1`,
		situationType, executorVersion).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read calibration artifact: %w", err)
	}
	return true, nil
}
