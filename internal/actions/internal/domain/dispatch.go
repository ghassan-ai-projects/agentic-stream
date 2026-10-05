package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// OutboxLease is the lease state of one command outbox row.
type OutboxLease struct{ Status, Owner, Until string }

// LeaseStanding reports whether owner still holds the outbox lease and, when
// it does, whether that lease is unexpired at now.
func (l OutboxLease) LeaseStanding(owner string, now time.Time) (held, live bool) {
	if l.Status != OutboxLeased || l.Owner != owner {
		return false, false
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, l.Until)
	return true, err == nil && expiresAt.After(now)
}

// DispatchResult is how one dispatch is recorded across the outcome, command,
// outbox and verification ledgers.
type DispatchResult struct {
	Status, Reconciliation, CommandStatus, ErrorCode string
	OutboxStatus, VerificationStatus                 string
	// Settled reports a final succeeded or failed outcome that needs no
	// further reconciliation.
	Settled bool
}

// ClassifyDispatch maps a provider effect and error to the ledger states.
func ClassifyDispatch(effect actionport.Effect, dispatchErr error) DispatchResult {
	result := DispatchResult{Status: OutcomeSucceeded, Reconciliation: ReconciliationObserved, CommandStatus: CommandSucceeded,
		OutboxStatus: OutboxDelivered, VerificationStatus: VerificationObserved}
	switch {
	case dispatchErr != nil && actionport.IsUnknownOutcome(dispatchErr):
		result.Status, result.Reconciliation, result.CommandStatus = OutcomeUnknown, ReconciliationRequired, CommandReconciling
		result.ErrorCode = ErrorOutcomeUnknown
	case dispatchErr != nil:
		result.Status, result.Reconciliation, result.CommandStatus = OutcomeFailed, ReconciliationNotRequired, CommandFailed
		result.ErrorCode = ErrorDispatchFailed
	case effect.VerificationPending:
		// A transport receipt is not physical success. Keep the command in the
		// existing non-terminal manual-review state until an independent
		// feedback verifier closes it; the outbox is delivered because no blind
		// resend is safe after the provider accepted the frame.
		result.Status, result.Reconciliation, result.CommandStatus = OutcomeReconcileRequired, ReconciliationRequired, CommandManualReview
	}
	return completeClassification(result, effect, dispatchErr)
}

func completeClassification(result DispatchResult, effect actionport.Effect, dispatchErr error) DispatchResult {
	if dispatchErr != nil {
		result.OutboxStatus = OutboxFailed
	}
	if dispatchErr != nil || effect.VerificationPending {
		result.VerificationStatus = VerificationAwaiting
	}
	result.Settled = !effect.VerificationPending && (result.Status == OutcomeSucceeded || result.Status == OutcomeFailed)
	return result
}

// ExpiredLeaseResult replaces a provider result that arrived after the lease
// expired: the next worker must not blindly repeat the effect.
func ExpiredLeaseResult() (actionport.Effect, error) {
	return actionport.Effect{}, &actionport.UnknownOutcomeError{Err: errors.New("lease expired before provider result")}
}

// OutcomeDocument is the schema document an outcome digest binds.
func OutcomeDocument(commandID, outcomeID, status string, result map[string]any, errorCode string, at time.Time) Document {
	document := Document{"outcome_id": outcomeID, "command_id": commandID, "status": status, "observed_at": at.UTC().Format(time.RFC3339Nano)}
	if result != nil {
		document["result"] = result
	}
	if errorCode != "" {
		document["error_code"] = errorCode
	}
	return document
}

// OutcomeDigest validates an outcome document against the shared schema and
// returns its raw digest.
func OutcomeDigest(document Document) ([]byte, error) {
	if err := contractsv1.Validate(contractsv1.SchemaOutcome, map[string]any(document)); err != nil {
		return nil, fmt.Errorf("validate outcome: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainOutcome, map[string]any(document))
	if err != nil {
		return nil, fmt.Errorf("digest outcome: %w", err)
	}
	outcomeSHA, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		return nil, fmt.Errorf("decode outcome digest: %w", err)
	}
	return outcomeSHA, nil
}

// NotifiedStatus is the outcome status the notification contract publishes: it
// calls an unverified physical result unknown, while the durable outcome keeps
// the more precise reconcile_required state for internal consumers.
func NotifiedStatus(status string) string {
	if status == OutcomeReconcileRequired {
		return OutcomeUnknown
	}
	return status
}

// DeviceCheck is a dispatch result after independent device-state verification.
type DeviceCheck struct {
	Effect      actionport.Effect
	DispatchErr error
	FinalStatus string
	Evidence    map[string]any
	VerifyErr   error
}

// ReconcilesUnknown reports whether verification settled an outcome the
// transport left unknown.
func (c DeviceCheck) ReconcilesUnknown() bool {
	return actionport.IsUnknownOutcome(c.DispatchErr) && c.FinalStatus != "" && c.VerifyErr == nil
}

// SkipsDeviceVerification reports whether a dispatch error rules out an
// independent device-state query: only a success or an unknown result is read.
func SkipsDeviceVerification(dispatchErr error) bool {
	return dispatchErr != nil && !actionport.IsUnknownOutcome(dispatchErr)
}

// ClassifyDeviceVerification folds a device-state query into a successful
// dispatch: a verification error makes it unknown, and a failed verification
// makes it failed.
func ClassifyDeviceVerification(check DeviceCheck) DeviceCheck {
	switch {
	case check.VerifyErr != nil:
		check.Effect.VerificationPending = false
		check.DispatchErr = &actionport.UnknownOutcomeError{Err: fmt.Errorf("verify device state: %w", check.VerifyErr)}
	case check.FinalStatus != "":
		check.Effect.VerificationPending = false
		if check.FinalStatus == CommandFailed {
			check.DispatchErr = errors.New("device state verification failed")
		}
	}
	return check
}
