package store

import (
	"context"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"time"
)

// Admit persists an episode through its lifecycle owner.
func (tx *Tx) Admit(ctx context.Context, admission episodeledger.Admission, now time.Time) error {
	return episodeledger.Admit(ctx, tx.tx, admission, now) //nolint:wrapcheck // Use cases preserve established operation context and sentinel errors.
}

// MarkAdmitted hands off the pending scheduler item atomically.
func (tx *Tx) MarkAdmitted(ctx context.Context, id string, now time.Time) error {
	return episodeledger.MarkSchedulerItemAdmitted(ctx, tx.tx, id, now) //nolint:wrapcheck // Use cases preserve established operation context and sentinel errors.
}

// StartAttemptOwned fences an attempt to the runtime owner.
func (tx *Tx) StartAttemptOwned(ctx context.Context, episode, attempt, owner string, now time.Time) (episodeledger.Identity, error) {
	return episodeledger.StartAttemptOwned(ctx, tx.tx, episode, attempt, owner, now) //nolint:wrapcheck // Use cases preserve established operation context and sentinel errors.
}

// StartAttempt allocates a fenced attempt for unowned fixture execution.
func (tx *Tx) StartAttempt(ctx context.Context, episode, attempt string, now time.Time) (episodeledger.Identity, error) {
	return episodeledger.StartAttempt(ctx, tx.tx, episode, attempt, now) //nolint:wrapcheck // Use cases preserve established operation context and sentinel errors.
}

// TransitionAttempt performs the lifecycle owner's fenced transition.
func (tx *Tx) TransitionAttempt(ctx context.Context, identity episodeledger.Identity, status episodeledger.AttemptStatus, now time.Time, terminal []byte) error {
	return episodeledger.TransitionAttempt(ctx, tx.tx, identity, status, now, terminal) //nolint:wrapcheck // Use cases preserve established operation context and sentinel errors.
}

// Rebind updates the episode's immutable snapshot binding.
func (tx *Tx) Rebind(ctx context.Context, id string, version int, digest, request []byte) error {
	return episodeledger.Rebind(ctx, tx.tx, id, version, digest, request) //nolint:wrapcheck // Use cases preserve established operation context and sentinel errors.
}

// BindRequest persists an attempt-bound request.
func (tx *Tx) BindRequest(ctx context.Context, id string, request []byte) error {
	return episodeledger.BindRequest(ctx, tx.tx, id, request) //nolint:wrapcheck // Use cases preserve established operation context and sentinel errors.
}

// RecordRejection records a worker or validation rejection.
func (tx *Tx) RecordRejection(ctx context.Context, identity episodeledger.Identity, reason episodeledger.RejectionReason, details []byte, now time.Time) error {
	return episodeledger.RecordRejection(ctx, tx.tx, identity, reason, details, now) //nolint:wrapcheck // Use cases preserve established operation context and sentinel errors.
}

// AbandonRebind records a failed rebind through the episode owner.
func (tx *Tx) AbandonRebind(ctx context.Context, id, now string, terminal []byte) error {
	return episodeledger.AbandonRebind(ctx, tx.tx, id, now, terminal) //nolint:wrapcheck // Use cases preserve established operation context and sentinel errors.
}

// Abandon quarantines an episode through its lifecycle owner.
func (tx *Tx) Abandon(ctx context.Context, id, now string, terminal []byte) error {
	return episodeledger.Abandon(ctx, tx.tx, id, now, terminal) //nolint:wrapcheck // Use cases preserve established operation context and sentinel errors.
}

// RetainForRetry leaves the episode eligible for another attempt.
func (tx *Tx) RetainForRetry(ctx context.Context, id string) error {
	return episodeledger.RetainForRetry(ctx, tx.tx, id) //nolint:wrapcheck // Use cases preserve established operation context and sentinel errors.
}

// Conclude persists an episode terminal through its owner.
func (tx *Tx) Conclude(ctx context.Context, id, now string, terminal []byte) error {
	return episodeledger.Conclude(ctx, tx.tx, id, now, terminal) //nolint:wrapcheck // Use cases preserve established operation context and sentinel errors.
}

// ReserveCost reserves the admitted episode budget on the same transaction.
func (tx *Tx) ReserveCost(ctx context.Context, controller *runtimecontrol.CostLedger, id, tenant string, budget uint64, now string) error {
	return controller.Reserve(ctx, tx.tx, id, tenant, budget, now) //nolint:wrapcheck // Use cases preserve established operation context and sentinel errors.
}

// SettleCost settles a reservation on the same transaction.
func (tx *Tx) SettleCost(ctx context.Context, controller *runtimecontrol.CostLedger, id string, cost uint64, now string) error {
	return controller.Settle(ctx, tx.tx, id, cost, now) //nolint:wrapcheck // Use cases preserve established operation context and sentinel errors.
}
