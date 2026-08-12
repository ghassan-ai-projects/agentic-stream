// Package runtime contains live-runtime composition primitives.
package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// RecoveryReport combines episode and evidence state repaired before runtime
// readiness. Both changes are committed with ownership in one transaction.
type RecoveryReport struct {
	Episodes            episodes.RecoveryReport
	InterruptedEvidence int
}

// RecoveryCoordinator claims a fresh runtime epoch and atomically recovers
// unfinished attempts and evidence calls. It does not expose readiness or
// start ingestion; the live command owns that sequencing.
type RecoveryCoordinator struct {
	Owner  *storage.RuntimeOwner
	Ledger *evidence.Ledger
	Epoch  string
	Now    func() time.Time
	Costs  *costcontrol.Controller
}

// ClaimAndRecover acquires ownership and commits all recovery mutations before
// returning. A failure rolls back ownership and all recovery changes.
func (c *RecoveryCoordinator) ClaimAndRecover(ctx context.Context) (RecoveryReport, error) {
	if c == nil || c.Owner == nil || c.Ledger == nil || c.Epoch == "" {
		return RecoveryReport{}, fmt.Errorf("runtime recovery is not configured")
	}
	if c.Ledger.RuntimeEpoch != c.Epoch {
		return RecoveryReport{}, fmt.Errorf("ledger runtime epoch does not match owner epoch")
	}
	c.Ledger.Owner = c.Owner
	now := time.Now().UTC()
	if c.Now != nil {
		now = c.Now().UTC()
	}
	var report RecoveryReport
	err := c.Owner.ClaimAndRecover(ctx, c.Epoch, func(tx *sql.Tx, claimedAt time.Time) error {
		if c.Now == nil {
			now = claimedAt
		}
		var err error
		report.Episodes, err = episodes.RecoverUnfinishedAttemptsWithCost(ctx, tx, c.Epoch, now, c.Costs)
		if err != nil {
			return fmt.Errorf("recover episode attempts: %w", err)
		}
		report.InterruptedEvidence, err = c.Ledger.RecoverTx(ctx, tx, now)
		if err != nil {
			return fmt.Errorf("recover evidence calls: %w", err)
		}
		return nil
	})
	if err != nil {
		return RecoveryReport{}, err
	}
	return report, nil
}
