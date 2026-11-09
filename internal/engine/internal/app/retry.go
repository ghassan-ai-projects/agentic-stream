package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

func (s *Service) RetryFailedReasoning(ctx context.Context, failed []domain.FailedReasoning) error {
	if s.spec.Cognition.RetryFailedReasoning == nil || len(failed) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, situationID := range domain.FailedSituations(failed) {
		if err := s.retryIfDue(ctx, situationID, failed); err != nil {
			return fmt.Errorf("retry the reasoning of situation %s: %w", situationID, err)
		}
	}
	return nil
}

func (s *Service) retryIfDue(ctx context.Context, situationID string, failed []domain.FailedReasoning) error {
	failedVersion, due, err := s.retryDue(ctx, situationID, failed)
	if err != nil || !due {
		return err
	}
	if err := s.wakeSituation(ctx, situationID, situations.WokenByFailedReasoning); err != nil {
		return err
	}
	s.retried[situationID] = failedVersion
	return nil
}

func (s *Service) retryDue(ctx context.Context, situationID string, failed []domain.FailedReasoning) (int, bool, error) {
	at, open := s.sitEngine.PositionOf(situationID)
	if !open {
		return 0, false, nil
	}
	reasoned, err := s.reasonedVersions(ctx, situationID, failed)
	if err != nil {
		return 0, false, err
	}
	failedVersion, due, err := domain.RetryDue(*s.spec.Cognition.RetryFailedReasoning, at, reasoned, s.retried[situationID])
	if err != nil {
		return 0, false, fmt.Errorf("judge the retry: %w", err)
	}
	return failedVersion, due, nil
}

func (s *Service) reasonedVersions(ctx context.Context, situationID string, failed []domain.FailedReasoning) ([]domain.ReasonedVersion, error) {
	var reasoned []domain.ReasonedVersion
	for _, reasoning := range failed {
		if reasoning.SituationID != situationID {
			continue
		}
		version, err := s.store.ReasonedVersion(ctx, situationID, reasoning.SituationVersion)
		if err != nil {
			return nil, fmt.Errorf("reasoned version: %w", err)
		}
		reasoned = append(reasoned, version)
	}
	return reasoned, nil
}
