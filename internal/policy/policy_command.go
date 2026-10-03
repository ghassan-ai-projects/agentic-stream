package policy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
)

// approveAutomatic approves an R0/R1 intent: it passes the interlock, gets
// exactly one command per intent, stays within the intent's hourly dispatch
// limit, and queues the command for the action plane.
func (g *Gateway) approveAutomatic(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any, result Result, now time.Time) (Result, error) {
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
func (g *Gateway) commandForIntent(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any, now time.Time) (commandDocument, string, error) {
	commandID, err := existingCommandID(ctx, tx, row.IntentID)
	if err != nil || commandID != "" {
		return commandDocument{}, commandID, err
	}
	command, err := newCommand(g, row, intent, now)
	if err != nil {
		return commandDocument{}, "", err
	}
	return storeCommandOnce(ctx, tx, row, command, now)
}

// rateLimited withdraws the prepared command and reports true when the
// intent type has used its hourly dispatch limit.
func (g *Gateway) rateLimited(ctx context.Context, tx *sql.Tx, row intentRow, commandID string, now time.Time) (bool, error) {
	if row.RateLimitPerHour <= 0 {
		return false, nil
	}
	overLimit, err := g.dispatchWithinLimit(ctx, tx, row, now)
	if err != nil || !overLimit {
		return false, err
	}
	if err := removePreparedCommand(ctx, tx, row.IntentID, commandID); err != nil {
		return false, err
	}
	return true, nil
}

// queueApprovedCommand writes the command outbox row, marks the intent
// approved, and audits the approval.
func (g *Gateway) queueApprovedCommand(ctx context.Context, tx *sql.Tx, row intentRow, command commandDocument, result Result, now time.Time) (Result, error) {
	if err := insertCommandOutbox(ctx, tx, command.ID, command.JSON, now); err != nil {
		return result, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE intents SET policy_status = 'approved', updated_at = ? WHERE intent_id = ?", formatTime(now), row.IntentID); err != nil {
		return result, fmt.Errorf("approve intent: %w", err)
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

func (g *Gateway) assertInterlock(ctx context.Context, tx *sql.Tx, row intentRow, intent map[string]any) error {
	if g.interlock == nil {
		return nil
	}
	if err := g.interlock.Assert(ctx, tx, row.TenantID, normalizedTarget(row.IntentID, intent), row.RiskClass); err != nil {
		return fmt.Errorf("assert action interlock: %w", err)
	}
	return nil
}

// existingCommandID returns an empty ID when the intent has no command. All
// other lookup failures are returned so callers cannot confuse missing data
// with a storage failure.
func existingCommandID(ctx context.Context, tx *sql.Tx, intentID string) (string, error) {
	var commandID string
	err := tx.QueryRowContext(ctx, "SELECT command_id FROM commands WHERE intent_id = ?", intentID).Scan(&commandID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find existing command: %w", err)
	}
	return commandID, nil
}

type commandDocument struct {
	ID     string
	JSON   []byte
	SHA    []byte
	Key    []byte
	Target string
}

func (g *Gateway) approvePreparedCommand(ctx context.Context, tx *sql.Tx, row intentRow, command commandDocument, existingID string, result Result, now time.Time) (Result, error) {
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

func storeCommandOnce(ctx context.Context, tx *sql.Tx, row intentRow, command commandDocument, now time.Time) (commandDocument, string, error) {
	inserted, err := insertCommand(ctx, tx, row, command, now)
	if err != nil || inserted {
		return command, "", err
	}
	commandID, err := existingCommandID(ctx, tx, row.IntentID)
	if err != nil {
		return commandDocument{}, "", err
	}
	if commandID == "" {
		return commandDocument{}, "", fmt.Errorf("command insert conflicted but no command exists for intent %s", row.IntentID)
	}
	return commandDocument{}, commandID, nil
}
