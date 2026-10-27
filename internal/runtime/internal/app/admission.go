package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

type AdmitterConfig struct {
	Store *store.PipelineStore
	Clock sources.Clock

	OwnerEpoch string

	EpochControl *runtimecontrol.EpochControl

	DemoMode bool
}

type Admitter struct {
	cfg AdmitterConfig
}

func NewAdmitter(cfg AdmitterConfig) (*Admitter, error) {
	if cfg.Store == nil || cfg.Store.Episodes == nil || cfg.Clock == nil {
		return nil, errors.New("admission store, episode assembler and clock are required")
	}
	return &Admitter{cfg: cfg}, nil
}

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

func (a *Admitter) admitNext(ctx context.Context) (admitted, more bool, err error) {
	now := a.cfg.Clock.Now().UTC()
	if a.draining(ctx) {
		return false, false, nil
	}
	poll, err := a.cfg.Store.PollSchedulerQueue(ctx, now)
	if err != nil {
		return false, false, err
	}
	if err := a.expireUnadmittable(ctx, poll.Expired, now); err != nil || !poll.Found {
		return false, false, err
	}
	return a.admitOrSkip(ctx, poll.Next, now)
}

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
	if waitsForTheLiveEpisode(admitErr) {
		return false, false, deferBehindTheLiveEpisode(ctx, itemID)
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

func deferBehindTheLiveEpisode(ctx context.Context, itemID string) error {
	slog.DebugContext(ctx, "episode admission deferred behind the situation's live episode", "scheduler_item_id", itemID)
	return nil
}

func waitsForTheLiveEpisode(err error) bool {
	return errors.Is(err, episodeledger.ErrLiveEpisodeBusy) || errors.Is(err, playbook.ErrAuditNotAdmittable)
}

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

func (a *Admitter) persistAdmission(ctx context.Context, tx *store.AdmissionTx, itemID string, now time.Time) (domain.AdmissionAttempt, error) {
	req, err := a.assembleOwned(ctx, tx, itemID)
	if err != nil {
		return domain.AdmissionAttempt{}, err
	}
	if err := domain.RefuseFixture(a.cfg.DemoMode, req.ExecutorName); err != nil {
		return domain.AdmissionAttempt{}, err
	}
	if err := a.routeThroughPlaybook(ctx, tx, req); err != nil {
		return attemptOf(req), err
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
