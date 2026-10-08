// Package store owns episode SQL and joins the original admission transaction.
package store

import (
	"context"
	"database/sql"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// OwnerCheck is a write fence run on the episode transaction.
type OwnerCheck = storage.OwnerCheck

// Store opens runner units of work and polls episode lifecycle state.
type Store struct {
	db    *storage.DB
	owner storage.OwnerCheck
}

// New wraps the episode database.
func New(db *storage.DB) Store { return Store{db: db} }

// Fenced binds the runtime owner check that owned attempts are fenced by.
func (s Store) Fenced(owner storage.OwnerCheck) Store {
	s.owner = ownerLostCheck(owner)
	return s
}

// IsFenced reports whether a runtime owner check was bound.
func (s Store) IsFenced() bool { return s.owner != nil }

// Tx joins a caller-owned transaction without exposing SQL to use cases.
type Tx struct {
	tx    *sql.Tx
	owner storage.OwnerCheck
}

// Join preserves the caller's transaction and its commit/rollback ownership.
func Join(tx *sql.Tx) *Tx { return &Tx{tx: tx} }

// ErrEpochUnbound identifies an episode without a decision epoch.
var ErrEpochUnbound = runtimecontrol.ErrEpochUnbound

// ErrEpochKilled identifies a killed decision epoch.
var ErrEpochKilled = runtimecontrol.ErrEpochKilled

// AssertDecisionEpoch invokes the configured check on the same transaction.
func (tx *Tx) AssertDecisionEpoch(ctx context.Context, check storage.OwnerCheck, epoch string) error {
	return check(ctx, tx.tx, epoch)
}

// WithTx runs a runner step in one owned transaction.
func (s Store) WithTx(ctx context.Context, work func(*Tx) error) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error { return work(&Tx{tx: tx, owner: s.owner}) })
}

// Configured reports whether the database adapter was supplied.
func (s Store) Configured() bool { return s.db != nil }
