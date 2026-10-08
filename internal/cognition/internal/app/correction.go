package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

// admitReconsiderations detects corrections that supersede a version whose
// accepted Intent produced a succeeded Command. The unique database key makes
// replay and duplicate correction delivery idempotent.
func (e *Service) admitReconsiderations(ctx context.Context, tx *store.Tx, current situations.Version) (int, error) {
	if !domain.ShouldReconsider(current, e.spec.Time.LatePolicy) {
		return 0, nil
	}
	if err := domain.RequirePolicyDigest(e.spec.Digest); err != nil {
		return 0, err
	}
	correction, correctionDigest, err := verifiedCorrection(ctx, tx, current)
	if err != nil {
		return 0, err
	}
	invalidated, err := tx.InvalidatedCommands(ctx, current)
	if err != nil {
		return 0, err
	}
	return e.admitInvalidated(ctx, tx, current, correction, correctionDigest, invalidated)
}

func verifiedCorrection(ctx context.Context, tx *store.Tx, current situations.Version) (map[string]any, []byte, error) {
	correction, err := domain.DecodeCorrection(current.SnapshotJSON)
	if err != nil {
		return nil, nil, err
	}
	persisted, err := tx.SnapshotDigest(ctx, current)
	if err != nil {
		return nil, nil, err
	}
	decoded, err := domain.MatchCorrectionDigest(correction, persisted)
	return correction, decoded, err
}

// admitInvalidated admits one reconsideration per invalidated command,
// counting only those not already admitted.
func (e *Service) admitInvalidated(ctx context.Context, tx *store.Tx, current situations.Version, correction map[string]any, correctionDigest []byte, invalidated []domain.InvalidatedCommand) (int, error) {
	admitted := 0
	for _, command := range invalidated {
		created, err := e.admitReconsideration(ctx, tx, current, correction, correctionDigest, command)
		if err != nil {
			return admitted, err
		}
		if created {
			admitted++
		}
	}
	return admitted, nil
}

// admitReconsideration records one reconsideration and admits its deep-lane
// episode. Identities derive from the Situation, superseded version, and
// command, so a duplicate correction delivery admits nothing.
func (e *Service) admitReconsideration(ctx context.Context, tx *store.Tx, current situations.Version, correction map[string]any, correctionDigest []byte, command domain.InvalidatedCommand) (bool, error) {
	exists, err := tx.ReconsiderationExists(ctx, current, command.CommandID)
	if err != nil || exists {
		return false, err
	}
	r := domain.NewReconsideration(current, command)
	deltaJSON, err := r.EvidenceJSON(correction)
	if err != nil {
		return false, e.rejectReconsideration(ctx, tx, r, err)
	}
	if err := e.persistReconsideration(ctx, tx, r, correctionDigest, deltaJSON); err != nil {
		return false, err
	}
	return true, nil
}

func (e *Service) rejectReconsideration(ctx context.Context, tx *store.Tx, r domain.Reconsideration, cause error) error {
	rejected := domain.RejectedReconsiderationEvaluation(r, cause, e.spec.Digest, e.clk.Now().UTC())
	if err := e.scheduler.saveEvaluation(ctx, tx, rejected, e.tenantID, e.deploymentID); err != nil {
		return fmt.Errorf("save rejected reconsideration evaluation: %w", err)
	}
	return nil
}

// persistReconsideration records the domain.Reconsideration, schedules its episode
// and announces it, in that order.
func (e *Service) persistReconsideration(ctx context.Context, tx *store.Tx, r domain.Reconsideration, correctionDigest, deltaJSON []byte) error {
	if err := tx.RecordReconsideration(ctx, r, correctionDigest, e.tenantID, e.clk.Now()); err != nil {
		return err
	}
	if err := e.scheduleReconsideration(ctx, tx, r, deltaJSON); err != nil {
		return err
	}
	return tx.AnnounceReconsideration(ctx, r, e.tenantID, e.clk.Now())
}

// scheduleReconsideration saves an admitted deep-lane evaluation carrying the
// domain.Reconsideration evidence and inserts its pending scheduler item.
func (e *Service) scheduleReconsideration(ctx context.Context, tx *store.Tx, r domain.Reconsideration, deltaJSON []byte) error {
	if err := e.scheduler.saveEvaluation(ctx, tx, domain.ReconsiderationEvaluation(r, deltaJSON, e.spec.Digest, e.clk.Now().UTC()), e.tenantID, e.deploymentID); err != nil {
		return fmt.Errorf("save reconsideration evaluation: %w", err)
	}
	if err := e.scheduler.insertItem(ctx, tx, domain.ReconsiderationItem(r, e.clk.Now().UTC()), e.tenantID); err != nil {
		return fmt.Errorf("insert reconsideration item: %w", err)
	}
	return tx.LinkReconsideration(ctx, r)
}
