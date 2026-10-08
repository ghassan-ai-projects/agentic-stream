package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
)

func (s *Service) revalidateAuthorization(ctx context.Context, leased domain.LeasedCommand) error {
	if err := s.store.WithTx(ctx, func(tx *store.Tx) error {
		return s.revalidateIn(ctx, tx, leased)
	}); err != nil {
		return fmt.Errorf("revalidate dispatch authorization: %w", err)
	}
	return nil
}

// revalidateIn re-proves, immediately before the effector call, that the leased
// command is still authorized: the lease is live, the command, intent and
// decision documents still match their digests and ledger rows, the intent is
// approved, unexpired and bound to the current Situation version, interlocks
// and R2 approvals still hold, and the policy digest is current. It then
// refreshes the lease.
func (s *Service) revalidateIn(ctx context.Context, tx *store.Tx, leased domain.LeasedCommand) error {
	if err := tx.AssertOwner(ctx); err != nil {
		return err
	}
	if err := s.requireLiveLease(ctx, tx, leased); err != nil {
		return err
	}
	records, err := tx.LoadAuthorizationRecords(ctx, leased.Command.CommandID)
	if err != nil {
		return err
	}
	commandDocument, err := records.VerifiedCommand()
	if err != nil {
		return err
	}
	return s.authorizeCurrentCommand(ctx, tx, leased, records, commandDocument)
}

func (s *Service) requireLiveLease(ctx context.Context, tx *store.Tx, leased domain.LeasedCommand) error {
	live, err := tx.LeaseIsLive(ctx, leased.OutboxID, leased.LeaseOwner, s.clk.Now())
	if err != nil {
		return err
	}
	if !live {
		return errors.New("dispatch lease is no longer active")
	}
	return nil
}

func (s *Service) authorizeCurrentCommand(ctx context.Context, tx *store.Tx, leased domain.LeasedCommand, records domain.AuthorizationRecords, commandDocument domain.CommandDocument) error {
	if err := records.RequireApprovedIntent(); err != nil {
		return err
	}
	if err := tx.AssertInterlock(ctx); err != nil {
		return err
	}
	if err := records.CheckApproval(s.clk.Now()); err != nil {
		return err
	}
	return s.authorizeCurrentDocuments(ctx, tx, leased, records, commandDocument)
}

func (s *Service) authorizeCurrentDocuments(ctx context.Context, tx *store.Tx, leased domain.LeasedCommand, records domain.AuthorizationRecords, commandDocument domain.CommandDocument) error {
	if err := records.RequireCurrent(); err != nil {
		return err
	}
	if err := records.CheckIntent(s.clk.Now()); err != nil {
		return err
	}
	if err := checkPolicyDigest(ctx, tx, commandDocument, records.Intent.ID); err != nil {
		return err
	}
	if err := records.CheckDecision(); err != nil {
		return err
	}
	return s.refreshLease(ctx, tx, leased)
}

// checkPolicyDigest requires a command that names a policy digest to match the
// latest approving policy evaluation of its intent.
func checkPolicyDigest(ctx context.Context, tx *store.Tx, commandDocument domain.CommandDocument, intentID string) error {
	commanded := commandDocument.PolicyDigest
	if commanded == "" {
		return nil
	}
	evaluated, err := tx.ApprovedPolicyDigest(ctx, intentID)
	if err != nil {
		return err
	}
	return domain.CheckPolicyDigest(commanded, evaluated)
}

func (s *Service) refreshLease(ctx context.Context, tx *store.Tx, leased domain.LeasedCommand) error {
	now := s.clk.Now()
	refreshed, err := tx.RefreshLease(ctx, leased.OutboxID, leased.LeaseOwner, now.Add(s.leaseFor), now)
	if err != nil {
		return err
	}
	if !refreshed {
		return errors.New("refresh dispatch lease lost ownership")
	}
	return nil
}
