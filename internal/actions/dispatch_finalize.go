package actions

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/notify"
)

func (d *Dispatcher) finalize(ctx context.Context, leased leasedCommand, effect Effect, dispatchErr error) error {
	if err := d.db.WithTx(ctx, func(tx *sql.Tx) error {
		return d.finalizeTx(ctx, tx, leased, effect, dispatchErr)
	}); err != nil {
		return fmt.Errorf("finalize command dispatch: %w", err)
	}
	return nil
}

// finalizeTx records the dispatch outcome while this dispatcher still holds
// the lease. A late result after another worker took the lease is dropped,
// and a result after this lease expired is recorded as unknown so the next
// worker cannot blindly repeat the effect.
func (d *Dispatcher) finalizeTx(ctx context.Context, tx *sql.Tx, leased leasedCommand, effect Effect, dispatchErr error) error {
	if err := d.assertRuntimeOwner(ctx, tx); err != nil {
		return err
	}
	now := d.clk.Now().UTC()
	held, live, err := leaseHeld(ctx, tx, leased, now)
	if err != nil || !held {
		return err
	}
	if !live {
		dispatchErr = &UnknownOutcomeError{Err: errors.New("lease expired before provider result")}
		effect = Effect{}
		if d.telemetry != nil {
			d.telemetry.ObserveLeaseExpiry()
		}
	}
	result := classifyDispatch(effect, dispatchErr)
	outcomeID := d.idGen.New(ids.PrefixOutcome)
	outcomeSHA, err := recordDispatchOutcome(ctx, tx, leased, effect, result, outcomeID, now)
	if err != nil {
		return err
	}
	if err := d.closeDispatch(ctx, tx, leased, result, outcomeID, now); err != nil {
		return err
	}
	return appendDispatchNotifications(ctx, tx, leased, result, outcomeID, outcomeSHA, now)
}

// leaseHeld reports whether this dispatcher still owns the outbox lease and,
// if so, whether that lease is unexpired at now.
func leaseHeld(ctx context.Context, tx *sql.Tx, leased leasedCommand, now time.Time) (held, live bool, err error) {
	var leaseStatus, leaseOwner, leaseUntil string
	if err := tx.QueryRowContext(ctx, `
		SELECT status, lease_owner, lease_until FROM outbox WHERE outbox_id = ?`,
		leased.OutboxID).Scan(&leaseStatus, &leaseOwner, &leaseUntil); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Another dispatcher owns the lease, or this worker's lease
			// expired. A late provider result must not overwrite it.
			return false, false, nil
		}
		return false, false, fmt.Errorf("verify command lease: %w", err)
	}
	if leaseStatus != "leased" || leaseOwner != leased.LeaseOwner {
		return false, false, nil
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, leaseUntil)
	return true, err == nil && expiresAt.After(now), nil
}

// dispatchResult is how one dispatch is recorded across the outcome,
// command, outbox, and verification ledgers.
type dispatchResult struct {
	status, reconciliation, commandStatus, errorCode string
	outboxStatus, verificationStatus                 string
	// settled reports a final succeeded or failed outcome that needs no
	// further reconciliation.
	settled bool
}

func classifyDispatch(effect Effect, dispatchErr error) dispatchResult {
	result := dispatchResult{status: "succeeded", reconciliation: "observed", commandStatus: "succeeded",
		outboxStatus: "delivered", verificationStatus: "observed"}
	switch {
	case dispatchErr != nil && IsUnknownOutcome(dispatchErr):
		result.status, result.reconciliation, result.commandStatus = "unknown", "required", "reconciling"
		result.errorCode = "outcome_unknown"
	case dispatchErr != nil:
		result.status, result.reconciliation, result.commandStatus = "failed", "not_required", "failed"
		result.errorCode = "dispatch_failed"
	case effect.VerificationPending:
		// A transport receipt is not physical success. Keep the command in the
		// existing non-terminal manual-review state until an independent
		// feedback verifier closes it; the outbox is delivered because no blind
		// resend is safe after the provider accepted the frame.
		result.status, result.reconciliation, result.commandStatus = "reconcile_required", "required", "manual_review"
	}
	if dispatchErr != nil {
		result.outboxStatus = "failed"
	}
	if dispatchErr != nil || effect.VerificationPending {
		result.verificationStatus = "awaiting"
	}
	result.settled = !effect.VerificationPending && (result.status == "succeeded" || result.status == "failed")
	return result
}

// outcomeDigest validates an outcome document against the shared schema and
// returns its raw digest.
func outcomeDigest(document map[string]any) ([]byte, error) {
	if err := contractsv1.Validate(contractsv1.SchemaOutcome, document); err != nil {
		return nil, fmt.Errorf("validate outcome: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainOutcome, document)
	if err != nil {
		return nil, fmt.Errorf("digest outcome: %w", err)
	}
	outcomeSHA, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		return nil, fmt.Errorf("decode outcome digest: %w", err)
	}
	return outcomeSHA, nil
}

func recordDispatchOutcome(ctx context.Context, tx *sql.Tx, leased leasedCommand, effect Effect, result dispatchResult, outcomeID string, now time.Time) ([]byte, error) {
	document := map[string]any{
		"outcome_id": outcomeID, "command_id": leased.Command.CommandID,
		"status": result.status, "observed_at": formatTime(now),
	}
	if effect.ProviderResult != nil {
		document["result"] = effect.ProviderResult
	}
	if result.errorCode != "" {
		document["error_code"] = result.errorCode
	}
	outcomeSHA, err := outcomeDigest(document)
	if err != nil {
		return nil, fmt.Errorf("action outcome: %w", err)
	}
	providerJSON, err := optionalJSON(effect.ProviderResult)
	if err != nil {
		return nil, fmt.Errorf("encode provider result: %w", err)
	}
	observedJSON, err := optionalJSON(effect.ObservedEffect)
	if err != nil {
		return nil, fmt.Errorf("encode observed effect: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO outcomes (
			outcome_id, command_id, ordinal, status, provider_result_json,
			observed_effect_json, reconciliation_status, outcome_sha256, traceparent, tracestate, occurred_at
		) VALUES (?, ?, (SELECT COALESCE(MAX(ordinal), 0) + 1 FROM outcomes WHERE command_id = ?), ?, ?, ?, ?, ?, ?, ?, ?)`,
		outcomeID, leased.Command.CommandID, leased.Command.CommandID, result.status, providerJSON,
		observedJSON, result.reconciliation, outcomeSHA, nullableString(leased.Traceparent), nullableString(leased.Tracestate), formatTime(now)); err != nil {
		return nil, fmt.Errorf("record action outcome: %w", err)
	}
	return outcomeSHA, nil
}

// closeDispatch moves the command, outbox, and verification rows to the
// states the dispatch result implies.
func (d *Dispatcher) closeDispatch(ctx context.Context, tx *sql.Tx, leased leasedCommand, result dispatchResult, outcomeID string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE commands SET status = ?, updated_at = ? WHERE command_id = ? AND status = 'dispatching'`,
		result.commandStatus, formatTime(now), leased.Command.CommandID); err != nil {
		return fmt.Errorf("record command status: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE outbox SET status = ?, lease_owner = NULL, lease_until = NULL,
			last_error_code = ?, delivered_at = CASE WHEN ? = 'delivered' THEN ? ELSE delivered_at END
		WHERE outbox_id = ? AND lease_owner = ?`,
		result.outboxStatus, nullableString(result.errorCode), result.outboxStatus, formatTime(now), leased.OutboxID, leased.LeaseOwner); err != nil {
		return fmt.Errorf("finish command outbox: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO verifications (verification_id, intent_id, command_id, outcome_id, status, updated_at)
		SELECT ?, intent_id, command_id, ?, ?, ? FROM commands WHERE command_id = ?
		ON CONFLICT(intent_id) DO UPDATE SET outcome_id = excluded.outcome_id,
			status = excluded.status, updated_at = excluded.updated_at`,
		d.idGen.New(ids.PrefixVerification), outcomeID, result.verificationStatus, formatTime(now), leased.Command.CommandID); err != nil {
		return fmt.Errorf("record command verification: %w", err)
	}
	return nil
}

func appendDispatchNotifications(ctx context.Context, tx *sql.Tx, leased leasedCommand, result dispatchResult, outcomeID string, outcomeSHA []byte, now time.Time) error {
	command := leased.Command
	trace := contractsv1.TraceContext{Traceparent: leased.Traceparent, Tracestate: leased.Tracestate}
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx, "command.dispatched:"+command.CommandID+":"+outcomeID, command.TenantID, notify.TypeCommandDispatched, "command/"+command.CommandID, command.CommandID, map[string]any{
		"tenant_id": command.TenantID, "command_id": command.CommandID, "intent_id": command.IntentID,
		"outcome_id": outcomeID, "status": result.commandStatus, "source_authority": notify.SourceForTenant(command.TenantID),
	}, now, trace); err != nil {
		return fmt.Errorf("append command dispatched notification: %w", err)
	}
	notificationStatus := result.status
	if notificationStatus == "reconcile_required" {
		// The notification contract deliberately calls an unverified physical
		// result unknown, while the durable outcome keeps the more precise
		// reconcile_required state for internal consumers.
		notificationStatus = "unknown"
	}
	if err := notify.AppendLifecycleEventWithTrace(ctx, tx, "outcome.recorded:"+outcomeID, command.TenantID, notify.TypeOutcomeRecorded, "outcome/"+outcomeID, command.CommandID, map[string]any{
		"tenant_id": command.TenantID, "outcome_id": outcomeID, "command_id": command.CommandID, "status": notificationStatus,
		"reconciliation_status": result.reconciliation, "intent_id": command.IntentID,
		"outcome_digest":   "sha256:" + hex.EncodeToString(outcomeSHA),
		"source_authority": notify.SourceForTenant(command.TenantID),
	}, now, trace); err != nil {
		return fmt.Errorf("append outcome recorded notification: %w", err)
	}
	if result.settled {
		if err := appendOutcomeReconciledNotification(ctx, tx, command.TenantID, command.IntentID, command.CommandID, outcomeID, result.status, now); err != nil {
			return fmt.Errorf("append outcome reconciled notification: %w", err)
		}
	}
	return nil
}
