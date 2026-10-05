package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
	"strings"
	"time"
)

func (g *Service) approveAutomatic(ctx context.Context, tx *store.Tx, row domain.IntentRecord, intent map[string]any, result domain.Result, now time.Time) (domain.Result, error) {
	if err := g.assertInterlock(ctx, tx, row, intent); err != nil {
		if !errors.Is(err, interlock.ErrTripped) {
			return result, err
		}
		return g.finishWithAuditReason(ctx, tx, row, result, "denied", "interlock_not_ready", interlockDenialAuditReason(err), now)
	}
	command, existingID, err := g.commandForIntent(ctx, tx, row, intent, now)
	if err != nil {
		return result, err
	}
	return g.approvePreparedCommand(ctx, tx, row, command, existingID, result, now)
}

// commandForIntent returns the ID of the intent's existing command, or
// inserts a new command and returns it. A concurrent insert that wins the
// race is reported as existing.
func (g *Service) commandForIntent(ctx context.Context, tx *store.Tx, row domain.IntentRecord, intent map[string]any, now time.Time) (domain.CommandRecord, string, error) {
	commandID, err := tx.ExistingCommandID(ctx, row.IntentID)
	if err != nil || commandID != "" {
		return domain.CommandRecord{}, commandID, err
	}
	command, err := domain.NewCommand(g.idGen.New(ids.PrefixCommand), g.policyDigest, row, intent, now)
	if err != nil {
		return domain.CommandRecord{}, "", err
	}
	return tx.StoreCommandOnce(ctx, row, command, now)
}

// rateLimited withdraws the prepared command and reports true when the
// intent type has used its hourly dispatch limit.
func (g *Service) rateLimited(ctx context.Context, tx *store.Tx, row domain.IntentRecord, commandID string, now time.Time) (bool, error) {
	if row.RateLimitPerHour <= 0 {
		return false, nil
	}
	overLimit, err := tx.DispatchWithinLimit(ctx, row, now)
	if err != nil || !overLimit {
		return false, err
	}
	if err := tx.RemovePreparedCommand(ctx, row.IntentID, commandID); err != nil {
		return false, err
	}
	return true, nil
}

// queueApprovedCommand writes the command outbox row, marks the intent
// approved, and audits the approval.
func (g *Service) queueApprovedCommand(ctx context.Context, tx *store.Tx, row domain.IntentRecord, command domain.CommandRecord, result domain.Result, now time.Time) (domain.Result, error) {
	if err := tx.InsertCommandOutbox(ctx, command.ID, command.JSON, now); err != nil {
		return result, err
	}
	if err := tx.SetIntentStatus(ctx, row.IntentID, "approved", now, store.ApproveIntentStatus); err != nil {
		return result, err
	}
	if result.Reason == "" {
		result.Reason = "automatic_r0_r1"
	}
	result.Result, result.CommandID = "approved", command.ID
	return g.audit(ctx, tx, row, result, result.Result, result.Reason, now)
}

func interlockDenialAuditReason(err error) string {
	const prefix = "assert action interlock: "
	detail := strings.TrimSpace(strings.TrimPrefix(err.Error(), prefix))
	if detail == "" {
		return "interlock_not_ready"
	}
	return "interlock_not_ready: " + detail
}

func (g *Service) assertInterlock(ctx context.Context, tx *store.Tx, row domain.IntentRecord, intent map[string]any) error {
	if err := tx.AssertInterlock(ctx, g.interlock, row.TenantID, domain.NormalizedTarget(row.IntentID, intent), row.RiskClass); err != nil {
		return fmt.Errorf("assert action interlock: %w", err)
	}
	return nil
}

// existingCommandID returns an empty ID when the intent has no command. All
// other lookup failures are returned so callers cannot confuse missing data
// with a storage failure.

func (g *Service) approvePreparedCommand(ctx context.Context, tx *store.Tx, row domain.IntentRecord, command domain.CommandRecord, existingID string, result domain.Result, now time.Time) (domain.Result, error) {
	if existingID != "" {
		result.Result, result.Reason, result.CommandID = "approved", "already_commanded", existingID
		return g.audit(ctx, tx, row, result, "approved", result.Reason, now)
	}
	limited, err := g.rateLimited(ctx, tx, row, command.ID, now)
	if err != nil {
		return result, err
	}
	if limited {
		return g.finish(ctx, tx, row, result, "denied", "rate_limited", now)
	}
	return g.queueApprovedCommand(ctx, tx, row, command, result, now)
}
