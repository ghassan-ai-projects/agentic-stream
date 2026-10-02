package actions

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
)

// lease claims the oldest available command for dispatch. A row whose
// earlier lease expired mid-dispatch is finalized as an unknown outcome, a
// command whose document no longer matches its ledger row is failed, and a
// command that already reached a terminal state only closes its outbox row.
// It reports found only when this dispatcher now holds the lease.
func (d *Dispatcher) lease(ctx context.Context) (leasedCommand, bool, error) {
	var leased leasedCommand
	found := false
	err := d.db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		leased, found, err = d.leaseTx(ctx, tx)
		return err
	})
	if err != nil {
		return leasedCommand{}, false, fmt.Errorf("lease command: %w", err)
	}
	return leased, found, nil
}

func (d *Dispatcher) leaseTx(ctx context.Context, tx *sql.Tx) (leasedCommand, bool, error) {
	if err := d.assertRuntimeOwner(ctx, tx); err != nil {
		return leasedCommand{}, false, err
	}
	now := d.clk.Now().UTC()
	candidate, err := loadDispatchCandidate(ctx, tx, now)
	if err != nil || candidate == nil {
		return leasedCommand{}, false, err
	}
	leased := candidate.leased
	if candidate.outboxStatus == "leased" {
		leased.LeaseOwner = candidate.leaseOwner.String
		if candidate.leaseExpired(now) {
			return leased, false, d.abandonExpiredLease(ctx, tx, leased, candidate)
		}
	}
	document, failureCode := candidate.verifiedDocument()
	if failureCode != "" {
		return leased, false, d.markLeaseFailure(ctx, tx, leased.OutboxID, leased.Command.CommandID, failureCode, now)
	}
	populateCommand(&leased.Command, document)
	if candidate.commandStatus == "succeeded" || candidate.commandStatus == "outcome_unknown" {
		return leased, false, d.finishOutboxOnly(ctx, tx, leased.OutboxID, candidate.commandStatus, now)
	}
	found, err := d.acquireLease(ctx, tx, &leased, now)
	return leased, found, err
}

// abandonExpiredLease records an unknown outcome for a command whose earlier
// lease expired mid-dispatch, so no worker can blindly repeat the effect.
func (d *Dispatcher) abandonExpiredLease(ctx context.Context, tx *sql.Tx, leased leasedCommand, candidate *dispatchCandidate) error {
	// Reclaimed rows have not gone through command-document population.
	// Finalization still emits tenant-scoped lifecycle records, so restore
	// the trusted ledger identity before recording the unknown outcome.
	leased.Command.TenantID = candidate.tenant
	leased.Command.IntentID = candidate.intentID
	if d.telemetry != nil {
		d.telemetry.ObserveLeaseExpiry()
	}
	return d.finalizeTx(ctx, tx, leased, actionport.Effect{}, &actionport.UnknownOutcomeError{Err: errors.New("lease expired before dispatch")})
}

// dispatchCandidate is the oldest available command outbox row with the
// ledger columns its document must match.
type dispatchCandidate struct {
	leased                                 leasedCommand
	commandStatus, intentID, tenant, route string
	target, outboxStatus                   string
	idempotency                            []byte
	leaseOwner, leaseUntil                 sql.NullString
}

func loadDispatchCandidate(ctx context.Context, tx *sql.Tx, now time.Time) (*dispatchCandidate, error) {
	var c dispatchCandidate
	var traceparent, tracestate sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT o.outbox_id, o.aggregate_id, c.command_json, c.command_sha256, c.status,
			c.intent_id, c.tenant_id, c.effector_route, c.normalized_target, c.idempotency_key,
			d.traceparent, d.tracestate
			, o.status, o.lease_owner, o.lease_until
		FROM outbox o
		JOIN commands c ON c.command_id = o.aggregate_id
		JOIN intents i ON i.intent_id = c.intent_id
		JOIN decisions d ON d.decision_id = i.decision_id
		WHERE o.kind = 'command'
		  AND o.available_at <= ?
		  AND (o.status = 'pending' OR (o.status = 'leased' AND o.lease_until <= ?))
		ORDER BY o.outbox_id
		LIMIT 1`, formatTime(now), formatTime(now)).Scan(
		&c.leased.OutboxID, &c.leased.Command.CommandID,
		&c.leased.CommandJSON, &c.leased.CommandSHA, &c.commandStatus,
		&c.intentID, &c.tenant, &c.route, &c.target, &c.idempotency,
		&traceparent, &tracestate,
		&c.outboxStatus, &c.leaseOwner, &c.leaseUntil,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find command outbox: %w", err)
	}
	c.leased.Traceparent = traceparent.String
	c.leased.Tracestate = tracestate.String
	return &c, nil
}

// leaseExpired reports whether a previously leased row has no valid,
// unexpired lease.
func (c *dispatchCandidate) leaseExpired(now time.Time) bool {
	expiresAt, err := time.Parse(time.RFC3339Nano, c.leaseUntil.String)
	return err != nil || !c.leaseOwner.Valid || !c.leaseUntil.Valid || !expiresAt.After(now)
}

// verifiedDocument decodes the command document and returns the lease
// failure code when it is invalid or disagrees with its ledger columns.
func (c *dispatchCandidate) verifiedDocument() (map[string]any, string) {
	var document map[string]any
	if err := json.Unmarshal(c.leased.CommandJSON, &document); err != nil {
		return nil, "command_json_invalid"
	}
	if err := contractsv1.Validate(contractsv1.SchemaCommand, document); err != nil {
		return nil, "command_schema_invalid"
	}
	providedIdempotency, idempotencyErr := canonicaljson.DecodeDigest(documentString(document, "idempotency_key"))
	if c.leased.Command.CommandID != documentString(document, "command_id") ||
		c.intentID != documentString(document, "intent_id") || c.tenant != documentString(document, "tenant_id") ||
		c.route != documentString(document, "effector_route") || c.target != documentString(document, "normalized_target") ||
		idempotencyErr != nil || !bytes.Equal(c.idempotency, providedIdempotency) ||
		!verifyDigest(canonicaljson.DomainCommand, document, c.leased.CommandSHA) {
		return nil, "command_digest_mismatch"
	}
	return document, ""
}

func populateCommand(command *actionport.Command, document map[string]any) {
	command.IntentID = documentString(document, "intent_id")
	command.TenantID = documentString(document, "tenant_id")
	command.EffectorRoute = documentString(document, "effector_route")
	command.NormalizedTarget = documentString(document, "normalized_target")
	command.IdempotencyKey = documentString(document, "idempotency_key")
	command.PolicyDigest = documentString(document, "policy_digest")
	command.NotBeforeMonoUS = documentInt64(document, "not_before_mono_us")
	command.Payload, _ = document["payload"].(map[string]any)
}

// acquireLease takes a fresh lease on the outbox row and marks the command
// dispatching. It reports false when another dispatcher won the row.
func (d *Dispatcher) acquireLease(ctx context.Context, tx *sql.Tx, leased *leasedCommand, now time.Time) (bool, error) {
	until := now.Add(d.leaseFor)
	leased.LeaseOwner = d.owner + "/" + d.idGen.New(ids.PrefixLease)
	result, err := tx.ExecContext(ctx, `
		UPDATE outbox
		SET status = 'leased', lease_owner = ?, lease_until = ?, attempt_count = attempt_count + 1
		WHERE outbox_id = ? AND (status = 'pending' OR (status = 'leased' AND lease_until <= ?))`,
		leased.LeaseOwner, formatTime(until), leased.OutboxID, formatTime(now))
	if err != nil {
		return false, fmt.Errorf("lease command outbox: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("count leased command outbox rows: %w", err)
	}
	if count != 1 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, "UPDATE commands SET status = 'dispatching', updated_at = ? WHERE command_id = ? AND status IN ('pending', 'failed', 'dispatching')", formatTime(now), leased.Command.CommandID); err != nil {
		return false, fmt.Errorf("mark command dispatching: %w", err)
	}
	return true, nil
}
