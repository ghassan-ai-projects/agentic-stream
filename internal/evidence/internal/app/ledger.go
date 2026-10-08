package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/wire"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// Ledger stores evidence-call reservations and completed bounded results.
// Capability bytes are never persisted.
type Ledger struct {
	Store        store.Store
	LeaseOwner   string
	RuntimeEpoch string
	Lease        time.Duration
	Now          func() time.Time
}

// Reserve claims one current, live evidence call.
func (l *Ledger) Reserve(ctx context.Context, call Call, tokenID, runtimeEpoch string) (ledgerReservation, error) {
	if l == nil || !l.Store.Configured() || l.LeaseOwner == "" || l.RuntimeEpoch == "" || runtimeEpoch == "" || runtimeEpoch != l.RuntimeEpoch || tokenID == "" {
		return ledgerReservation{}, fmt.Errorf("evidence ledger is not configured")
	}
	now, lease := l.now(), sources.OrLease(l.Lease)
	fingerprint, err := wire.CallFingerprint(call)
	if err != nil {
		return ledgerReservation{}, err
	}
	key := ledgerKey{TenantID: call.TenantID, EpisodeID: call.EpisodeID, AttemptID: call.AttemptID, Fence: call.Fence, CallID: call.CallID}
	pending := ledgerReservation{Key: key, RequestSHA256: fingerprint, TokenID: tokenID, RuntimeEpoch: runtimeEpoch, Status: "running", Created: true}
	return l.reserveCall(ctx, call, pending, now, lease)
}
func (l *Ledger) now() time.Time {
	now := time.Now().UTC()
	if l.Now != nil {
		now = l.Now().UTC()
	}
	return now
}
func (l *Ledger) reserveCall(ctx context.Context, call Call, pending ledgerReservation, now time.Time, lease time.Duration) (ledgerReservation, error) {
	var reservation ledgerReservation
	err := l.Store.WithTx(ctx, func(tx *store.Tx) error {
		var err error
		reservation, err = l.reserveTx(ctx, tx, call, pending, now, lease)
		return err
	})
	if err != nil {
		return ledgerReservation{}, fmt.Errorf("reserve evidence call transaction: %w", err)
	}
	return reservation, nil
}
func (l *Ledger) reserveTx(ctx context.Context, tx *store.Tx, call Call, pending ledgerReservation, now time.Time, lease time.Duration) (ledgerReservation, error) {
	if err := l.assertOwner(ctx, tx); err != nil {
		return ledgerReservation{}, err
	}
	if err := assertLiveAttempt(ctx, tx, call); err != nil {
		return ledgerReservation{}, err
	}
	existing, err := loadReservation(ctx, tx, pending)
	if err != nil || existing != nil {
		return derefReservation(existing), err
	}
	if err := tx.InsertReservation(ctx, call, pending, now, lease, l.LeaseOwner); err != nil {
		return ledgerReservation{}, err
	}
	return pending, nil
}
func derefReservation(reservation *ledgerReservation) ledgerReservation {
	if reservation == nil {
		return ledgerReservation{}
	}
	return *reservation
}
func (l *Ledger) assertOwner(ctx context.Context, tx *store.Tx) error { return tx.AssertOwner(ctx) }
func assertLiveAttempt(ctx context.Context, tx *store.Tx, call Call) error {
	return assertAttempt(ctx, call, tx.LiveEpisode, domain.CheckLiveEpisode, tx.LiveAttempt, domain.CheckLiveAttempt)
}

func assertAttempt[K any](ctx context.Context, key K, loadEpisode func(context.Context, K) (domain.EpisodeState, error), checkEpisode func(domain.EpisodeState, K) error,
	loadAttempt func(context.Context, K) (bool, error), checkAttempt func(bool) error) error {
	state, err := loadEpisode(ctx, key)
	if err != nil {
		return err
	}
	if err := checkEpisode(state, key); err != nil {
		return err
	}
	inFlight, err := loadAttempt(ctx, key)
	if err != nil {
		return err
	}
	return checkAttempt(inFlight)
}
func loadReservation(ctx context.Context, tx *store.Tx, pending ledgerReservation) (*ledgerReservation, error) {
	row, err := tx.ReadReservation(ctx, pending.Key)
	if err != nil || row == nil {
		return nil, err
	}
	return domain.ExistingReservation(*row, pending.Key, pending.RequestSHA256, pending.TokenID, pending.RuntimeEpoch)
}
