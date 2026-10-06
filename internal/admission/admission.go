package admission

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/scheduleledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// ErrFixtureRejected is returned when a production route (no demo mode)
// admits a scheduler item whose executor is `fixture`.
var ErrFixtureRejected = errors.New("fixture executor rejected")

// Config is the owner-scoped admission composition for one tenant.
type Config struct {
	DB         *storage.DB
	Episodes   *episodes.Service
	Clock      clock.Clock
	TenantID   string
	Owner      *runtimecontrol.RuntimeOwner
	OwnerEpoch string
	// EpochControl, when set, stops new admissions while the epoch drains.
	EpochControl *runtimecontrol.EpochControl
	// DemoMode admits `fixture` executors (demos and tests only).
	DemoMode bool
}

// Admitter admits pending scheduler items in queue order.
type Admitter struct {
	cfg Config
}

// New creates an admitter for the configured tenant and runtime owner.
func New(cfg Config) *Admitter {
	return &Admitter{cfg: cfg}
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
	itemID, found, err := scheduleledger.NextPending(ctx, a.cfg.DB.DB, a.cfg.TenantID, now)
	if err != nil || !found {
		return false, false, err //nolint:wrapcheck // The ledger names the failed read; the batch error text is unchanged.
	}
	return a.admitOrSkip(ctx, itemID, now)
}

// draining reports whether the epoch is draining or killed. No NEW episode is
// admitted then; in-flight episodes finish under their recorded epoch. This
// is a stop, not a batch error, so the runner keeps draining the backlog.
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

// attempt identifies the request an admission assembled, so a refusal can be
// reported against it.
type attempt struct {
	kind, situationID string
}

// admitItem assembles and persists one episode for a scheduler item,
// stamping the current policy epoch exactly once.
func (a *Admitter) admitItem(ctx context.Context, itemID string, now time.Time) (attempt, error) {
	var assembled attempt
	err := a.cfg.DB.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		assembled, err = a.persistAdmission(ctx, tx, itemID, now)
		return err
	})
	if err != nil {
		return assembled, fmt.Errorf("persist episode admission: %w", err)
	}
	return assembled, nil
}

// persistAdmission assembles the item's request, refuses a production
// fixture, stamps the owner epoch and persists the episode.
func (a *Admitter) persistAdmission(ctx context.Context, tx *sql.Tx, itemID string, now time.Time) (attempt, error) {
	req, err := a.assembleOwned(ctx, tx, itemID)
	if err != nil {
		return attempt{}, err
	}
	if err := a.refuseFixture(req); err != nil {
		return attempt{}, err
	}
	req.PolicyEpoch = a.cfg.OwnerEpoch
	assembled := attempt{kind: req.Kind, situationID: req.SituationID}
	return assembled, a.cfg.Episodes.Persist(ctx, tx, req, now) //nolint:wrapcheck // Wrapped by admitItem with the admission step.
}

// assembleOwned fences the transaction to the runtime owner, then assembles
// the scheduler item's episode request.
func (a *Admitter) assembleOwned(ctx context.Context, tx *sql.Tx, itemID string) (*episodes.Request, error) {
	if err := a.assertOwner(ctx, tx); err != nil {
		return nil, fmt.Errorf("assert pipeline owner: %w", err)
	}
	req, err := a.cfg.Episodes.Assemble(ctx, tx, itemID, a.cfg.TenantID)
	if err != nil {
		return nil, fmt.Errorf("assemble scheduler item: %w", err)
	}
	return req, nil
}

// refuseFixture rejects the `fixture` executor on a production route: it
// exists for demos and tests only, never on a live route.
func (a *Admitter) refuseFixture(req *episodes.Request) error {
	if !a.cfg.DemoMode && req.ExecutorName == "fixture" {
		return fmt.Errorf("%w: fixture executor %s on a production route", ErrFixtureRejected, req.ExecutorName)
	}
	return nil
}

func (a *Admitter) assertOwner(ctx context.Context, tx *sql.Tx) error {
	if a.cfg.Owner == nil || a.cfg.OwnerEpoch == "" {
		return nil
	}
	if err := a.cfg.Owner.Assert(ctx, tx, a.cfg.OwnerEpoch); err != nil {
		return fmt.Errorf("runtime ownership lost: %w", err)
	}
	return nil
}
