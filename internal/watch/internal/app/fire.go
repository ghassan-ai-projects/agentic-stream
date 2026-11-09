package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/watch/internal/store"
)

type FeatureSource func(ctx context.Context, situationID string) (map[string]any, bool, error)

func (s *Service) FireEvent(ctx context.Context, eventID, target string, source FeatureSource) ([]string, error) {
	if eventID == "" || target == "" {
		return nil, errors.New("watch event identity is required")
	}
	candidates, err := s.store.ActiveForTarget(ctx, target, s.clk.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("active watches of target %s: %w", target, err)
	}
	return s.fireCandidates(ctx, eventID, target, source, candidates)
}

func (s *Service) fireCandidates(ctx context.Context, eventID, target string, source FeatureSource, candidates []domain.Candidate) ([]string, error) {
	var fired []string
	for _, item := range candidates {
		matched, err := s.fireCandidate(ctx, eventID, target, source, item)
		if err != nil {
			return fired, err
		}
		if matched {
			fired = append(fired, item.SituationID)
		}
	}
	return fired, nil
}

func (s *Service) fireCandidate(ctx context.Context, eventID, target string, source FeatureSource, item domain.Candidate) (bool, error) {
	features, found, err := source(ctx, item.SituationID)
	if err != nil {
		return false, fmt.Errorf("features of situation %s for watch %s: %w", item.SituationID, item.WatchID, err)
	}
	if !found {
		slog.WarnContext(ctx, "watch skipped: its situation holds no runtime state", "watch_id", item.WatchID, "event_id", eventID, "situation_id", item.SituationID)
		return false, nil
	}
	return s.fire(ctx, item.WatchID, eventID, item.SituationID, target, features)
}

func (s *Service) fire(ctx context.Context, watchID, eventID, situationID, target string, features map[string]any) (bool, error) {
	if watchID == "" || eventID == "" {
		return false, errors.New("watch identity is required")
	}
	now := s.clk.Now().UTC()
	var fired bool
	err := s.store.WithTx(ctx, func(tx *store.Tx) error {
		var err error
		fired, err = fireIn(ctx, tx, watchID, eventID, situationID, target, features, now)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("watch fire transaction: %w", err)
	}
	return fired, nil
}

func fireIn(ctx context.Context, tx *store.Tx, watchID, eventID, situationID, target string, features map[string]any, now time.Time) (bool, error) {
	if err := assertGuards(ctx, tx, "", ""); err != nil {
		return false, fmt.Errorf("guard the watch fire: %w", err)
	}
	if err := tx.ExpireDue(ctx, now); err != nil {
		return false, fmt.Errorf("expire due watches before firing: %w", err)
	}
	active, found, err := tx.LoadActive(ctx, watchID, now)
	if err != nil {
		return false, fmt.Errorf("load active watch %s: %w", watchID, err)
	}
	if !found || !active.InScope(situationID, target) {
		return false, nil
	}
	rose, err := evaluationRises(ctx, tx, watchID, eventID, active, features, now)
	if err != nil || !rose {
		return false, err
	}
	return recordFire(ctx, tx, watchID, eventID, now)
}

func evaluationRises(ctx context.Context, tx *store.Tx, watchID, eventID string, active domain.ActiveWatch, features map[string]any, now time.Time) (bool, error) {
	matches := matchesFeatures(ctx, watchID, eventID, active, features)
	if !active.Changes(matches) {
		return false, nil
	}
	if err := tx.RecordEvaluation(ctx, watchID, matches, now); err != nil {
		return false, fmt.Errorf("record the evaluation of watch %s: %w", watchID, err)
	}
	return active.RisesTo(matches), nil
}

func matchesFeatures(ctx context.Context, watchID, eventID string, active domain.ActiveWatch, features map[string]any) bool {
	matches, err := domain.Evaluate(active.Expression, features)
	if err != nil {
		slog.WarnContext(ctx, "watch expression evaluation skipped",
			"watch_id", watchID, "event_id", eventID, "situation_id", active.SituationID, "target", active.Target, "error", err)
		return false
	}
	return matches
}

func recordFire(ctx context.Context, tx *store.Tx, watchID, eventID string, now time.Time) (bool, error) {
	recorded, err := tx.RecordFire(ctx, watchID, eventID, now)
	if err != nil {
		return false, fmt.Errorf("record the fire of watch %s: %w", watchID, err)
	}
	if !recorded {
		return false, nil
	}
	if err := tx.SpendAllowance(ctx, watchID, now); err != nil {
		return false, fmt.Errorf("spend the allowance of watch %s: %w", watchID, err)
	}
	return true, nil
}
