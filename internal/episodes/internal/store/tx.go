// Package store owns episode SQL and joins the original admission transaction.
package store

import (
	"context"
	"database/sql"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Store opens runner units of work and polls episode lifecycle state.
type Store struct{ db *storage.DB }

// New wraps the episode database.
func New(db *storage.DB) Store { return Store{db: db} }

// Tx joins a caller-owned transaction without exposing SQL to use cases.
type Tx struct{ tx *sql.Tx }

// Join preserves the caller's transaction and its commit/rollback ownership.
func Join(tx *sql.Tx) *Tx { return &Tx{tx: tx} }

// ErrNoRows is the empty-projection sentinel.
var ErrNoRows = sql.ErrNoRows

// ErrEpochUnbound identifies an episode without a decision epoch.
var ErrEpochUnbound = runtimecontrol.ErrEpochUnbound

// ErrEpochKilled identifies a killed decision epoch.
var ErrEpochKilled = runtimecontrol.ErrEpochKilled

// DecisionEpochCheck asserts an episode's epoch on the original transaction.
type DecisionEpochCheck func(context.Context, *sql.Tx, string) error

// AssertDecisionEpoch invokes the configured check on the same transaction.
func (tx *Tx) AssertDecisionEpoch(ctx context.Context, check DecisionEpochCheck, epoch string) error {
	return check(ctx, tx.tx, epoch)
}

// WithTx runs a runner step in one owned transaction.
func (s Store) WithTx(ctx context.Context, work func(*Tx) error) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error { return work(Join(tx)) })
}

// Configured reports whether the database adapter was supplied.
func (s Store) Configured() bool { return s.db != nil }
