package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// RecoveryReport reports the episode and evidence repairs committed together.
type RecoveryReport = domain.RecoveryReport

// RecoveryCoordinator claims a fresh runtime epoch and atomically recovers
// unfinished attempts and evidence calls. It does not expose readiness or
// start ingestion; the live command owns that sequencing.
type RecoveryCoordinator struct {
	Owner  *runtimecontrol.RuntimeOwner
	Ledger *evidence.Service
	Epoch  string
	Now    func() time.Time
	Costs  *runtimecontrol.CostLedger
}

// ClaimAndRecover acquires ownership and commits all recovery mutations before
// returning. A failure rolls back ownership and all recovery changes.
func (c *RecoveryCoordinator) ClaimAndRecover(ctx context.Context) (RecoveryReport, error) {
	if c == nil || c.Owner == nil || c.Ledger == nil || c.Epoch == "" {
		return RecoveryReport{}, fmt.Errorf("runtime recovery is not configured")
	}
	if c.Ledger.RuntimeEpoch() != c.Epoch {
		return RecoveryReport{}, fmt.Errorf("ledger runtime epoch does not match owner epoch")
	}
	return c.claimRecovery(ctx)
}

func (c *RecoveryCoordinator) claimRecovery(ctx context.Context) (RecoveryReport, error) {
	now := sources.NowUTC(c.Now)
	var report RecoveryReport
	err := c.Owner.ClaimAndRecover(ctx, c.Epoch, func(tx *sql.Tx, claimedAt time.Time) error {
		if c.Now == nil {
			now = claimedAt
		}
		var err error
		report, err = c.recoverLedgers(ctx, tx, now)
		return err
	})
	if err != nil {
		return RecoveryReport{}, fmt.Errorf("claim and recover runtime: %w", err)
	}
	return report, nil
}

// costSettler is the configured cost ledger, or nothing when recovery has none
// (a nil ledger must not become a non-nil port).
func (c *RecoveryCoordinator) costSettler() episodeledger.CostSettler {
	if c.Costs == nil {
		return nil
	}
	return c.Costs
}

func (c *RecoveryCoordinator) recoverLedgers(ctx context.Context, tx *sql.Tx, now time.Time) (RecoveryReport, error) {
	episodes, err := episodeledger.RecoverUnfinishedAttempts(ctx, tx, c.Epoch, now, c.costSettler())
	if err != nil {
		return RecoveryReport{}, fmt.Errorf("recover episode attempts: %w", err)
	}
	interrupted, err := c.Ledger.RecoverTx(ctx, tx, now)
	if err != nil {
		return RecoveryReport{}, fmt.Errorf("recover evidence calls: %w", err)
	}
	return RecoveryReport{Episodes: domain.EpisodeRecoveryReport(episodes), InterruptedEvidence: interrupted}, nil
}
