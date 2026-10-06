package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// AdmitterConfig is the owner-scoped admission composition for one tenant.
type AdmitterConfig struct {
	// Store carries the database, the runtime owner, the episode assembler and the tenant.
	Store *store.PipelineStore
	Clock sources.Clock
	// OwnerEpoch is the policy epoch every admitted episode is stamped with.
	OwnerEpoch string
	// EpochControl, when set, stops new admissions while the epoch drains.
	EpochControl *runtimecontrol.EpochControl
	// DemoMode admits `fixture` executors (demos and tests only).
	DemoMode bool
}

// Admitter admits pending scheduler items into epoch-stamped episodes in queue order.
type Admitter struct {
	cfg AdmitterConfig
}

// NewAdmitter creates an admitter; the store and the clock are required.
func NewAdmitter(cfg AdmitterConfig) (*Admitter, error) {
	if cfg.Store == nil || cfg.Store.Episodes == nil || cfg.Clock == nil {
		return nil, errors.New("admission store, episode assembler and clock are required")
	}
	return &Admitter{cfg: cfg}, nil
}

// AdmitPending admits every due pending scheduler item, skipping the ones
// that can never be admitted, and returns how many episodes it admitted.
func (a *Admitter) AdmitPending(ctx context.Context) (int, error) {
	count := 0
	for {
		admitted, more, err := a.admitNext(ctx)
		if err != nil || !more {
			return count, err
		}
		if admitted {
			count++
		}
	}
}

// admitNext admits or skips the next due item. more is false once the queue
// is empty or the epoch is draining.
func (a *Admitter) admitNext(ctx context.Context) (admitted, more bool, err error) {
	now := a.cfg.Clock.Now().UTC()
	if a.draining(ctx) {
		return false, false, nil
	}
	itemID, found, err := a.cfg.Store.NextPendingSchedulerItem(ctx, now)
	if err != nil || !found {
		return false, false, err
	}
	return a.admitOrSkip(ctx, itemID, now)
}

// draining reports whether the epoch is draining or killed. No NEW episode is
// admitted then; in-flight episodes finish under their recorded epoch. This is
// a stop, not a batch error, so the runner keeps draining the backlog.
func (a *Admitter) draining(ctx context.Context) bool {
	if a.cfg.EpochControl == nil {
		return false
	}
	return a.cfg.EpochControl.AssertAdmission(ctx, a.cfg.OwnerEpoch) != nil
}

func (a *Admitter) admitOrSkip(ctx context.Context, itemID string, now time.Time) (admitted, more bool, err error) {
	attempt, admitErr := a.admitItem(ctx, itemID, now)
	if admitErr == nil {
		return true, true, nil
	}
	skipped, skipErr := a.skipUnadmittable(ctx, itemID, now, attempt, admitErr)
	if skipErr != nil {
		return false, false, skipErr
	}
	if !skipped {
		return false, false, fmt.Errorf("admit scheduler item %s: %w", itemID, admitErr)
	}
	return false, true, nil
}

// admitItem assembles and persists one episode for a scheduler item,
// stamping the current policy epoch exactly once.
func (a *Admitter) admitItem(ctx context.Context, itemID string, now time.Time) (domain.AdmissionAttempt, error) {
	var assembled domain.AdmissionAttempt
	err := a.cfg.Store.InAdmission(ctx, func(tx *store.AdmissionTx) error {
		var err error
		assembled, err = a.persistAdmission(ctx, tx, itemID, now)
		return err
	})
	if err != nil {
		return assembled, fmt.Errorf("persist episode admission: %w", err)
	}
	return assembled, nil
}

// persistAdmission fences the transaction to the runtime owner, assembles the
// item's request, refuses a production fixture, stamps the owner epoch and
// persists the episode.
func (a *Admitter) persistAdmission(ctx context.Context, tx *store.AdmissionTx, itemID string, now time.Time) (domain.AdmissionAttempt, error) {
	req, err := a.assembleOwned(ctx, tx, itemID)
	if err != nil {
		return domain.AdmissionAttempt{}, err
	}
	if err := domain.RefuseFixture(a.cfg.DemoMode, req.ExecutorName); err != nil {
		return domain.AdmissionAttempt{}, err
	}
	req.PolicyEpoch = a.cfg.OwnerEpoch
	return attemptOf(req), tx.Persist(ctx, req, now)
}

func (a *Admitter) assembleOwned(ctx context.Context, tx *store.AdmissionTx, itemID string) (*episodes.Request, error) {
	if err := tx.AssertOwner(ctx); err != nil {
		return nil, fmt.Errorf("assert pipeline owner: %w", err)
	}
	return tx.Assemble(ctx, itemID)
}

func attemptOf(req *episodes.Request) domain.AdmissionAttempt {
	return domain.AdmissionAttempt{Kind: req.Kind, SituationID: req.SituationID}
}
