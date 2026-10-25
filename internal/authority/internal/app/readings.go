package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/store"
)

func (s *Service) ResolveWithReadings(ctx context.Context, evidence domain.ReadingEvidence) (bool, error) {
	if err := s.checkReadingEvidence(evidence); err != nil {
		return false, err
	}
	now := s.now()
	var cleared bool
	err := s.inAdmittedTx(ctx, evidence.Owner.Epoch, func(tx *store.Tx) (err error) {
		cleared, err = s.resolveWithReadings(ctx, tx, evidence, now)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("resolve device reconciliation with readings: %w", err)
	}
	return cleared, nil
}

func (s *Service) checkReadingEvidence(evidence domain.ReadingEvidence) error {
	if !evidence.Complete() {
		return errors.New("device boot, owner, source, state and outputs are required to resolve with readings")
	}
	return s.checkOwner(evidence.Owner)
}

func (s *Service) resolveWithReadings(ctx context.Context, tx *store.Tx, evidence domain.ReadingEvidence, now time.Time) (bool, error) {
	opening, opened, err := tx.LatestOpening(ctx, evidence.Device)
	if err != nil || !opened {
		return false, err
	}
	request, clears, err := evidence.Resolution(opening)
	if err != nil || !clears {
		return false, err
	}
	return true, s.resolveReconciliation(ctx, tx, request, now)
}
