package app

import (
	"context"
	"errors"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

func (g *Service) approveAutomatic(ctx context.Context, tx *store.Tx, e evaluation) (domain.Result, error) {
	if err := g.assertInterlock(ctx, tx, e); err != nil {
		if !errors.Is(err, interlock.ErrTripped) {
			return e.result, err
		}
		return g.finish(ctx, tx, e, domain.Outcome{Status: "denied", Reason: "interlock_not_ready", AuditDetail: interlockDenialAuditReason(err)})
	}
	command, existingID, err := g.commandForIntent(ctx, tx, e)
	if err != nil {
		return e.result, err
	}
	return g.approvePreparedCommand(ctx, tx, e, command, existingID)
}

// commandForIntent returns the ID of the intent's existing command, or
// inserts a new command and returns it. A concurrent insert that wins the
// race is reported as existing.
func (g *Service) commandForIntent(ctx context.Context, tx *store.Tx, e evaluation) (domain.CommandRecord, string, error) {
	commandID, err := tx.ExistingCommandID(ctx, e.row.IntentID)
	if err != nil || commandID != "" {
		return domain.CommandRecord{}, commandID, err
	}
	command, err := domain.NewCommand(domain.CommandPreparation{ID: g.idGen.New(sources.PrefixCommand), PolicyDigest: g.policyDigest, Row: e.row, Intent: e.documents.Intent, Now: e.now})
	if err != nil {
		return domain.CommandRecord{}, "", err
	}
	return tx.StoreCommandOnce(ctx, e.row, command, e.now)
}

// rateLimited withdraws the prepared command and reports true when the
// intent type has used its hourly dispatch limit.
func (g *Service) rateLimited(ctx context.Context, tx *store.Tx, e evaluation, commandID string) (bool, error) {
	if e.row.RateLimitPerHour <= 0 {
		return false, nil
	}
	overLimit, err := tx.DispatchWithinLimit(ctx, e.row, e.now)
	if err != nil || !overLimit {
		return false, err
	}
	if err := tx.RemovePreparedCommand(ctx, e.row.IntentID, commandID); err != nil {
		return false, err
	}
	return true, nil
}

// queueApprovedCommand writes the command outbox row, marks the intent
// approved, and audits the approval.
func (g *Service) queueApprovedCommand(ctx context.Context, tx *store.Tx, e evaluation, command domain.CommandRecord) (domain.Result, error) {
	if err := tx.InsertCommandOutbox(ctx, command.ID, command.JSON, e.now); err != nil {
		return e.result, err
	}
	if err := tx.SetIntentStatus(ctx, domain.IntentStatusChange{IntentID: e.row.IntentID, Status: "approved", Now: e.now, Operation: store.ApproveIntentStatus}); err != nil {
		return e.result, err
	}
	if e.result.Reason == "" {
		e.result.Reason = "automatic_r0_r1"
	}
	e.result.Result, e.result.CommandID = "approved", command.ID
	return g.audit(ctx, tx, e, domain.Outcome{Status: e.result.Result, Reason: e.result.Reason})
}

func interlockDenialAuditReason(err error) string {
	_, detail, found := strings.Cut(err.Error(), interlock.ErrTripped.Error()+": ")
	detail = strings.TrimSpace(detail)
	if !found || detail == "" {
		return "interlock_not_ready"
	}
	return "interlock_not_ready: " + detail
}

func (g *Service) assertInterlock(ctx context.Context, tx *store.Tx, e evaluation) error {
	return tx.AssertInterlock(ctx)
}

func (g *Service) approvePreparedCommand(ctx context.Context, tx *store.Tx, e evaluation, command domain.CommandRecord, existingID string) (domain.Result, error) {
	if existingID != "" {
		e.result.Result, e.result.Reason, e.result.CommandID = "approved", "already_commanded", existingID
		return g.audit(ctx, tx, e, domain.Outcome{Status: "approved", Reason: e.result.Reason})
	}
	limited, err := g.rateLimited(ctx, tx, e, command.ID)
	if err != nil {
		return e.result, err
	}
	if limited {
		return g.finish(ctx, tx, e, domain.Outcome{Status: "denied", Reason: "rate_limited"})
	}
	return g.queueApprovedCommand(ctx, tx, e, command)
}
