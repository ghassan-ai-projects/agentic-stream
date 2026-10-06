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

// FireEvent evaluates all active watches scoped to one event target. The event
// log remains the source of evidence; duplicate event delivery is absorbed by
// the fire record's identity.
func (s *Service) FireEvent(ctx context.Context, eventID, target string, features map[string]any) (int, error) {
	if eventID == "" || target == "" {
		return 0, errors.New("watch event identity is required")
	}
	candidates, err := s.store.ActiveForTarget(ctx, target, s.clk.Now().UTC())
	if err != nil {
		return 0, err
	}
	return s.fireCandidates(ctx, eventID, target, features, candidates)
}

func (s *Service) fireCandidates(ctx context.Context, eventID, target string, features map[string]any, candidates []domain.Candidate) (int, error) {
	fired := 0
	for _, item := range candidates {
		matched, err := s.fire(ctx, item.WatchID, eventID, item.SituationID, target, features)
		if err != nil {
			return fired, err
		}
		if matched {
			fired++
		}
	}
	return fired, nil
}

// fire records one event-driven watch firing exactly once and spends its
// bounded allowance. It returns false for expired, disabled, duplicate, or
// expression-evaluation-error no-fires.
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
		return false, err
	}
	if err := tx.ExpireDue(ctx, now); err != nil {
		return false, err
	}
	matches, err := activeWatchMatches(ctx, tx, watchID, eventID, situationID, target, features, now)
	if err != nil || !matches {
		return false, err
	}
	return recordFire(ctx, tx, watchID, eventID, now)
}

// activeWatchMatches reports whether an active, unexpired watch scoped to this
// Situation and target matches the features. An expression that fails to
// evaluate is a logged no-fire, never an error.
func activeWatchMatches(ctx context.Context, tx *store.Tx, watchID, eventID, situationID, target string, features map[string]any, now time.Time) (bool, error) {
	active, found, err := tx.LoadActive(ctx, watchID, now)
	if err != nil || !found || !active.InScope(situationID, target) {
		return false, err
	}
	matches, err := domain.Evaluate(active.Expression, features)
	if err != nil {
		slog.WarnContext(ctx, "watch expression evaluation skipped",
			"watch_id", watchID, "event_id", eventID, "situation_id", active.SituationID, "target", active.Target, "error", err)
		return false, nil
	}
	return matches, nil
}

// recordFire records the fire at most once per event and spends one unit of the
// watch's allowance.
func recordFire(ctx context.Context, tx *store.Tx, watchID, eventID string, now time.Time) (bool, error) {
	recorded, err := tx.RecordFire(ctx, watchID, eventID, now)
	if err != nil || !recorded {
		return false, err
	}
	if err := tx.SpendAllowance(ctx, watchID, now); err != nil {
		return false, err
	}
	return true, nil
}
