package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/actions/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// reconcileUnknown closes an outcome_unknown command using independently
// observed provider evidence. It is the only path that may resolve a command
// after an effector call whose result was uncertain.
func (s *Service) reconcileUnknown(ctx context.Context, commandID, finalStatus string, evidence map[string]any) error {
	parsed, err := domain.ParseReconciliation(finalStatus, evidence)
	if err != nil {
		return err //nolint:wrapcheck // The domain rule names the refused evidence.
	}
	if err := s.store.WithTx(ctx, func(tx *store.Tx) error {
		return s.reconcileIn(ctx, tx, commandID, finalStatus, parsed)
	}); err != nil {
		return fmt.Errorf("reconcile unknown command: %w", err)
	}
	return nil
}

func (s *Service) reconcileIn(ctx context.Context, tx *store.Tx, commandID, finalStatus string, evidence domain.Evidence) error {
	if err := tx.AssertOwner(ctx); err != nil {
		return err
	}
	command, err := tx.LoadReconcilableCommand(ctx, commandID)
	if err != nil {
		return err
	}
	if err := command.RequireAwaitingReconciliation(); err != nil {
		return err
	}
	if err := tx.VerifyDeviceBinding(ctx, command, evidence.Raw); err != nil {
		return err
	}
	return s.recordReconciliation(ctx, tx, command, finalStatus, evidence)
}

func (s *Service) recordReconciliation(ctx context.Context, tx *store.Tx, command domain.ReconcilableCommand, finalStatus string, evidence domain.Evidence) error {
	now := s.clk.Now().UTC()
	outcomeID := s.ids.New(sources.PrefixOutcome)
	digest, err := domain.OutcomeDigest(domain.ReconciliationOutcomeDocument(command.ID, outcomeID, finalStatus, evidence.Raw, now))
	if err != nil {
		return fmt.Errorf("reconciliation outcome: %w", err)
	}
	outcome := domain.OutcomeRecord{ID: outcomeID, CommandID: command.ID, Status: domain.OutcomeReconciled, Reconciliation: domain.ReconciliationReconciled,
		ProviderResult: evidence.Raw, SHA: digest, Trace: command.Trace, At: now}
	if err := tx.InsertOutcome(ctx, outcome); err != nil {
		return err
	}
	return closeReconciliation(ctx, tx, domain.ReconciliationClosure{Command: command, FinalStatus: finalStatus, OutcomeID: outcomeID, At: now})
}

func closeReconciliation(ctx context.Context, tx *store.Tx, closure domain.ReconciliationClosure) error {
	if err := tx.CloseReconciliation(ctx, closure); err != nil {
		return err
	}
	command := closure.Command
	return appendReconciliationNotice(ctx, tx, command.TenantID, command.IntentID, command.ID, closure.OutcomeID, closure.FinalStatus, closure.At)
}

// appendReconciliationNotice publishes the settled outcome, citing the stored
// outcome row it reconciled.
func appendReconciliationNotice(ctx context.Context, tx *store.Tx, tenantID, intentID, commandID, outcomeID, finalStatus string, at time.Time) error {
	provenance, err := tx.LoadReconciledProvenance(ctx, outcomeID, commandID, intentID)
	if err != nil {
		return err
	}
	if err := provenance.Validate(); err != nil {
		return err
	}
	return tx.AppendReconciliationNotice(ctx, domain.ReconciliationNotice{TenantID: tenantID, IntentID: intentID, CommandID: commandID,
		OutcomeID: outcomeID, FinalStatus: finalStatus, Provenance: provenance, At: at})
}
