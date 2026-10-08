package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/store"
)

func (l *Ledger) Complete(ctx context.Context, reservation ledgerReservation, result QueryResult) error {
	if l == nil || !l.Store.Configured() {
		return fmt.Errorf("evidence ledger is not configured")
	}
	now := l.now()
	// Persist even when the caller was canceled; keep its values (trace).
	persistenceCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	err := l.Store.WithTx(persistenceCtx, func(tx *store.Tx) error { return l.completeTx(persistenceCtx, tx, reservation, result, now) })
	if err != nil {
		return fmt.Errorf("complete evidence call transaction: %w", err)
	}
	return nil
}

func (l *Ledger) completeTx(ctx context.Context, tx *store.Tx, reservation ledgerReservation, result QueryResult, now time.Time) error {
	if err := l.assertOwner(ctx, tx); err != nil {
		return err
	}
	if err := assertAttemptRunning(ctx, tx, reservation.Key); err != nil {
		return err
	}
	return tx.StoreResult(ctx, reservation, result, now, l.LeaseOwner)
}

func (l *Ledger) Fail(ctx context.Context, reservation ledgerReservation, code string) error {
	if l == nil || !l.Store.Configured() {
		return fmt.Errorf("evidence ledger is not configured")
	}
	// Persist even when the caller was canceled; keep its values (trace).
	persistenceCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	now := l.now()
	err := l.Store.WithTx(persistenceCtx, func(tx *store.Tx) error { return l.failTx(persistenceCtx, tx, reservation, code, now) })
	if err != nil {
		return fmt.Errorf("fail evidence call transaction: %w", err)
	}
	return nil
}

func (l *Ledger) ReclaimExpired(ctx context.Context, now time.Time) error {
	if l == nil || !l.Store.Configured() || l.RuntimeEpoch == "" {
		return fmt.Errorf("evidence ledger is not configured")
	}
	err := l.Store.WithTx(ctx, func(tx *store.Tx) error { return l.reclaimExpiredTx(ctx, tx, now) })
	if err != nil {
		return fmt.Errorf("reclaim evidence call ledger: %w", err)
	}
	return nil
}

func (l *Ledger) RecoverTx(ctx context.Context, tx *store.Tx, now time.Time) (int, error) {
	if l == nil || !tx.Configured() || l.RuntimeEpoch == "" {
		return 0, fmt.Errorf("evidence ledger recovery is not configured")
	}
	if err := l.assertOwner(ctx, tx); err != nil {
		return 0, err
	}
	return tx.Recover(ctx, now)
}
func assertAttemptRunning(ctx context.Context, tx *store.Tx, key ledgerKey) error {
	return assertAttempt(ctx, key, tx.CompletionEpisode, domain.CheckCompletionEpisode, tx.CompletionAttempt, domain.CheckCompletionAttempt)
}
func (l *Ledger) failTx(ctx context.Context, tx *store.Tx, reservation ledgerReservation, code string, now time.Time) error {
	if err := l.assertOwner(ctx, tx); err != nil {
		return err
	}
	return tx.Fail(ctx, reservation, code, now, l.LeaseOwner)
}
func (l *Ledger) reclaimExpiredTx(ctx context.Context, tx *store.Tx, now time.Time) error {
	if err := l.assertOwner(ctx, tx); err != nil {
		return err
	}
	return tx.ReclaimExpired(ctx, now)
}
