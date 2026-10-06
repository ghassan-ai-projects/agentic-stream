package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
)

func (s *Service) finalize(ctx context.Context, leased domain.LeasedCommand, effect actionport.Effect, dispatchErr error) error {
	if err := s.store.WithTx(ctx, func(tx *store.Tx) error {
		return s.finalizeIn(ctx, tx, leased, effect, dispatchErr)
	}); err != nil {
		return fmt.Errorf("finalize command dispatch: %w", err)
	}
	return nil
}

// finalizeIn records the dispatch outcome while this dispatcher still holds the
// lease. A late result after another worker took the lease is dropped, and a
// result after this lease expired is recorded as unknown so the next worker
// cannot blindly repeat the effect.
func (s *Service) finalizeIn(ctx context.Context, tx *store.Tx, leased domain.LeasedCommand, effect actionport.Effect, dispatchErr error) error {
	if err := tx.AssertOwner(ctx); err != nil {
		return err
	}
	now := s.clk.Now().UTC()
	lease, found, err := tx.LoadOutboxLease(ctx, leased.OutboxID)
	if err != nil || !found {
		return err
	}
	held, live := lease.LeaseStanding(leased.LeaseOwner, now)
	if !held {
		return nil
	}
	effect, dispatchErr = s.resultAtStanding(live, effect, dispatchErr)
	return s.recordFinalDispatch(ctx, tx, leased, effect, dispatchErr, now)
}

// resultAtStanding keeps the provider result while the lease is live and
// replaces it with an unknown outcome once the lease has expired.
func (s *Service) resultAtStanding(live bool, effect actionport.Effect, dispatchErr error) (actionport.Effect, error) {
	if live {
		return effect, dispatchErr
	}
	s.observeLeaseExpiry()
	return domain.ExpiredLeaseResult()
}

func (s *Service) recordFinalDispatch(ctx context.Context, tx *store.Tx, leased domain.LeasedCommand, effect actionport.Effect, dispatchErr error, now time.Time) error {
	result := domain.ClassifyDispatch(effect, dispatchErr)
	outcome, err := s.dispatchOutcome(leased, effect, result, now)
	if err != nil {
		return err
	}
	if err := tx.InsertOutcome(ctx, outcome); err != nil {
		return err
	}
	closure := domain.DispatchClosure{Leased: leased, Result: result, OutcomeID: outcome.ID, VerificationID: s.ids.New(ids.PrefixVerification), At: now}
	if err := tx.CloseDispatch(ctx, closure); err != nil {
		return err
	}
	return appendDispatchNotices(ctx, tx, closure, outcome)
}

func (s *Service) dispatchOutcome(leased domain.LeasedCommand, effect actionport.Effect, result domain.DispatchResult, now time.Time) (domain.OutcomeRecord, error) {
	outcomeID := s.ids.New(ids.PrefixOutcome)
	document := domain.OutcomeDocument(leased.Command.CommandID, outcomeID, result.Status, effect.ProviderResult, result.ErrorCode, now)
	digest, err := domain.OutcomeDigest(document)
	if err != nil {
		return domain.OutcomeRecord{}, fmt.Errorf("action outcome: %w", err)
	}
	return domain.OutcomeRecord{ID: outcomeID, CommandID: leased.Command.CommandID, Status: result.Status, Reconciliation: result.Reconciliation,
		ProviderResult: effect.ProviderResult, ObservedEffect: effect.ObservedEffect, SHA: digest, Trace: leased.Trace, At: now}, nil
}

func appendDispatchNotices(ctx context.Context, tx *store.Tx, closure domain.DispatchClosure, outcome domain.OutcomeRecord) error {
	command, result := closure.Leased.Command, closure.Result
	if err := tx.AppendDispatchNotice(ctx, domain.DispatchNotice{Command: command, Trace: closure.Leased.Trace, Status: result.CommandStatus, OutcomeID: outcome.ID, At: closure.At}); err != nil {
		return err
	}
	if err := tx.AppendOutcomeNotice(ctx, domain.OutcomeNotice{Command: command, Trace: closure.Leased.Trace, OutcomeID: outcome.ID, Status: outcome.Status,
		Reconciliation: outcome.Reconciliation, Digest: outcome.SHA, At: closure.At}); err != nil {
		return err
	}
	if !result.Settled {
		return nil
	}
	return appendReconciliationNotice(ctx, tx, command.TenantID, command.IntentID, command.CommandID, outcome.ID, result.Status, closure.At)
}
