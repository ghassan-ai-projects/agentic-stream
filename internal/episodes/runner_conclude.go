package episodes

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
)

// recordExecution persists the result of one executed attempt: a failed or
// mismatched execution fails the attempt, and a usable outcome concludes it.
func (r *Runner) recordExecution(ctx context.Context, claim *episodeClaim, outcome *Outcome, executionErr error, deadlineExceeded bool) error {
	identity := claim.identity
	switch {
	case executionErr != nil:
		return r.failAttemptStatus(ctx, identity, executionFailureStatus(executionErr), executionFailureReason(executionErr))
	case outcome == nil:
		return r.failAttemptStatus(ctx, identity, AttemptFailed, "executor_returned_nil_outcome")
	case outcome.AttemptID != identity.AttemptID || outcome.Fence != identity.Fence:
		reason := RejectWrongAttempt
		if outcome.Fence < identity.Fence {
			reason = RejectStaleAttempt
		}
		incoming := Identity{EpisodeID: identity.EpisodeID, AttemptID: outcome.AttemptID, Fence: outcome.Fence}
		return r.failAttemptWithRejection(ctx, identity, incoming, reason, "worker_identity_mismatch")
	case deadlineExceeded:
		// P8 (freshness): a decision that arrives after the episode's
		// wall_time deadline is refused. The deadline is never extended to let
		// a slow model pass, and an in-process executor cannot bypass it.
		return r.failAttemptStatus(ctx, identity, AttemptTimedOut, "decision_after_deadline")
	default:
		return r.concludeAttempt(ctx, claim, outcome)
	}
}

// concludeAttempt persists a usable outcome in one transaction: the proposed
// Decision, its governance or rejection, and the attempt and episode terminals.
func (r *Runner) concludeAttempt(ctx context.Context, claim *episodeClaim, outcome *Outcome) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		now := r.runtimeNow()
		// P8 (kill, post-execute): the recorded epoch is re-asserted INSIDE
		// the persistence transaction. An attempt dispatched before a kill
		// that completes after it is refused here, so a hostile worker cannot
		// slip a decision into governance.
		reason, err := r.epochRefusal(ctx, tx, claim.req.PolicyEpoch)
		if err != nil {
			return fmt.Errorf("check post-execute policy epoch: %w", err)
		}
		if reason != "" {
			return r.abandonAfterExecute(ctx, tx, claim, outcome, reason+"_post_execute", now)
		}
		record, err := r.persistDecision(ctx, tx, claim, outcome, now)
		if err != nil {
			return err
		}
		return r.finishAttempt(ctx, tx, claim, outcome, record, now)
	})
}

// abandonAfterExecute quarantines an episode whose epoch was killed or
// unbound while its attempt ran.
func (r *Runner) abandonAfterExecute(ctx context.Context, tx *sql.Tx, claim *episodeClaim, outcome *Outcome, reason string, now string) error {
	attemptTerminal, err := json.Marshal(map[string]any{
		"status": string(AttemptAbandoned),
		"reason": reason,
	})
	if err != nil {
		return fmt.Errorf("marshal post-execute attempt terminal: %w", err)
	}
	if err := TransitionAttempt(ctx, tx, claim.identity, AttemptAbandoned, r.clk.Now(), attemptTerminal); err != nil {
		return fmt.Errorf("abandon killed epoch attempt: %w", err)
	}
	if err := r.abandonEpisode(ctx, tx, claim.episodeID, map[string]any{"reason": reason}, now); err != nil {
		return fmt.Errorf("quarantine post-execute killed-epoch episode: %w", err)
	}
	if r.cost != nil {
		if err := r.cost.Settle(ctx, tx, claim.episodeID, outcome.CostMicrounits, now); err != nil {
			return fmt.Errorf("settle post-execute quarantined episode cost: %w", err)
		}
	}
	return nil
}

// decisionRecord is a proposed Decision as validated and stored.
type decisionRecord struct {
	id             string
	digest         []byte
	validated      *decisions.Result
	validationErr  error
	validationJSON []byte
}

// persistDecision validates and stores the outcome's Decision, then sends a
// valid one to governance and records why an invalid one was rejected. It
// returns nil when the outcome carries no Decision.
func (r *Runner) persistDecision(ctx context.Context, tx *sql.Tx, claim *episodeClaim, outcome *Outcome, now string) (*decisionRecord, error) {
	if outcome.DecisionJSON == nil {
		return nil, nil
	}
	record, err := r.validateDecision(claim, outcome)
	if err != nil {
		return nil, err
	}
	if err := insertDecision(ctx, tx, claim, outcome, record, now); err != nil {
		return nil, err
	}
	if record.validationErr != nil {
		return record, r.rejectDecision(ctx, tx, claim.identity, record)
	}
	return record, r.governDecision(ctx, tx, claim, outcome, record, now)
}

// validateDecision checks the Decision against the episode's contract and
// derives the identity and digest it is stored under.
func (r *Runner) validateDecision(claim *episodeClaim, outcome *Outcome) (*decisionRecord, error) {
	validationInput, err := decisionInput(&claim.req, claim.identity, r.clk.Now())
	if err != nil {
		return nil, fmt.Errorf("build decision validation input: %w", err)
	}
	record := &decisionRecord{}
	record.validated, record.validationErr = decisions.Validate(outcome.DecisionJSON, outcome.DecisionSHA256, validationInput)
	record.id = decisionIDFromJSON(outcome.DecisionJSON)
	if record.id == "" {
		record.id = r.idGen.New(ids.PrefixDecision)
	}
	digest, hasContractDigest := decisionDigestForStorage(outcome.DecisionJSON)
	if !hasContractDigest {
		rawHash := sha256.Sum256(outcome.DecisionJSON)
		digest = rawHash[:]
	}
	record.digest = digest
	if record.validationErr == nil {
		record.validationJSON = []byte(`{}`)
		if decoded, decodeErr := canonicaljson.DecodeDigest(record.validated.DecisionDigest); decodeErr == nil {
			record.digest = decoded
		}
		return record, nil
	}
	record.validationJSON, err = validationFailureJSON(record.validationErr, outcome.DecisionJSON, hasContractDigest)
	if err != nil {
		return nil, err
	}
	return record, nil
}

// validationFailureJSON describes a rejected Decision. Without a contract
// digest the raw bytes' hash is recorded so the rejected input stays traceable.
func validationFailureJSON(validationErr error, raw []byte, hasContractDigest bool) ([]byte, error) {
	var validationJSON []byte
	var err error
	var typed *decisions.ValidationError
	if errors.As(validationErr, &typed) {
		validationJSON, err = json.Marshal(map[string]any{"reason": typed.Reason, "details": typed.Details})
	} else {
		validationJSON, err = json.Marshal(map[string]any{"reason": "schema_invalid", "details": validationErr.Error()})
	}
	if err != nil {
		return nil, fmt.Errorf("marshal decision validation: %w", err)
	}
	if hasContractDigest {
		return validationJSON, nil
	}
	rawHash := sha256.Sum256(raw)
	var details map[string]any
	if err := json.Unmarshal(validationJSON, &details); err != nil {
		return nil, fmt.Errorf("decode decision validation: %w", err)
	}
	details["raw_sha256"] = hex.EncodeToString(rawHash[:])
	validationJSON, err = json.Marshal(details)
	if err != nil {
		return nil, fmt.Errorf("marshal raw decision validation: %w", err)
	}
	return validationJSON, nil
}

func insertDecision(ctx context.Context, tx *sql.Tx, claim *episodeClaim, outcome *Outcome, record *decisionRecord, now string) error {
	validationStatus := "rejected"
	if record.validationErr == nil {
		validationStatus = "proposed"
	}
	var ordinal int
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(ordinal), 0) + 1 FROM decisions WHERE episode_id = ?", claim.episodeID).Scan(&ordinal); err != nil {
		return fmt.Errorf("allocate decision ordinal: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO decisions (
			decision_id, episode_id, attempt_id, fence, ordinal, situation_id,
			situation_version, raw_json, decision_sha256, validation_status,
			validation_json, traceparent, tracestate, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.id, claim.episodeID, claim.identity.AttemptID, claim.identity.Fence, ordinal,
		claim.req.SituationID, claim.req.SituationVersion,
		outcome.DecisionJSON, record.digest, validationStatus, record.validationJSON,
		nullableString(claim.req.Traceparent), nullableString(claim.req.Tracestate), now,
	); err != nil {
		return fmt.Errorf("insert decision: %w", err)
	}
	return nil
}

// governDecision hands a valid Decision to the action plane, or only scores it
// in shadow mode, and marks it accepted.
func (r *Runner) governDecision(ctx context.Context, tx *sql.Tx, claim *episodeClaim, outcome *Outcome, record *decisionRecord, now string) error {
	if claim.req.DispatchPolicy == "shadow" {
		// P8 (shadow-first): a shadow decision is scored (the would-be policy
		// outcome is computed from the intents) but NOTHING is written to
		// intents or commands. Shadow never enters action governance.
		if err := r.recordShadow(ctx, tx, record.id, record.digest, &claim.req, outcome, record.validated, now); err != nil {
			return err
		}
	} else if err := r.persistValidatedIntents(ctx, tx, record.validated, &claim.req, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE decisions SET validation_status = 'accepted' WHERE decision_id = ?", record.id); err != nil {
		return fmt.Errorf("accept decision: %w", err)
	}
	return nil
}

func (r *Runner) rejectDecision(ctx context.Context, tx *sql.Tx, identity Identity, record *decisionRecord) error {
	reason := "schema_invalid"
	var typed *decisions.ValidationError
	if errors.As(record.validationErr, &typed) {
		reason = typed.Reason
	}
	if err := RecordRejection(ctx, tx, identity, RejectionReason(reason), record.validationJSON, r.clk.Now()); err != nil {
		return fmt.Errorf("record decision rejection: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE decisions SET rejection_reason = ? WHERE decision_id = ?", reason, record.id); err != nil {
		return fmt.Errorf("annotate rejected decision: %w", err)
	}
	return nil
}

// finishAttempt records the attempt terminal, settles cost, and concludes the
// episode.
func (r *Runner) finishAttempt(ctx context.Context, tx *sql.Tx, claim *episodeClaim, outcome *Outcome, record *decisionRecord, now string) error {
	attemptStatus := terminalAttemptStatus(outcome, record)
	if !IsTerminalAttempt(attemptStatus) {
		return fmt.Errorf("executor returned non-terminal attempt status %q", attemptStatus)
	}
	terminalJSON, err := json.Marshal(outcome)
	if err != nil {
		return fmt.Errorf("marshal outcome: %w", err)
	}
	if err := TransitionAttempt(ctx, tx, claim.identity, attemptStatus, r.clk.Now(), terminalJSON); err != nil {
		return fmt.Errorf("finish episode attempt: %w", err)
	}
	if r.cost != nil {
		if err := r.cost.Settle(ctx, tx, claim.identity.EpisodeID, outcome.CostMicrounits, now); err != nil {
			return fmt.Errorf("settle episode cost: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE episodes SET lifecycle_status = 'concluded', ended_at = ?, terminal_json = ?
		WHERE episode_id = ?`,
		now, terminalJSON, claim.episodeID,
	); err != nil {
		return fmt.Errorf("update episode terminal: %w", err)
	}
	return nil
}

// terminalAttemptStatus derives the attempt terminal: a Decision makes it
// produced or failed by validation; otherwise the executor's status stands,
// and an empty status means the executor declined.
func terminalAttemptStatus(outcome *Outcome, record *decisionRecord) AttemptStatus {
	switch {
	case record != nil && record.validationErr == nil:
		return AttemptProduced
	case record != nil:
		return AttemptFailed
	case outcome.Status == "":
		return AttemptDeclined
	default:
		return AttemptStatus(outcome.Status)
	}
}

// failAttemptStatus fails the current attempt, then retries the episode or
// concludes it once the retry budget is spent.
func (r *Runner) failAttemptStatus(ctx context.Context, identity Identity, attemptStatus AttemptStatus, reason string) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		terminalJSON, err := json.Marshal(map[string]any{"status": attemptStatus, "reason": reason})
		if err != nil {
			return fmt.Errorf("marshal terminal: %w", err)
		}
		if attemptStatus == AttemptCancelled {
			if err := r.markCancelling(ctx, tx, identity); err != nil {
				return err
			}
		}
		if err := TransitionAttempt(ctx, tx, identity, attemptStatus, r.clk.Now(), terminalJSON); err != nil {
			return fmt.Errorf("finish failed episode attempt: %w", err)
		}
		return r.retryOrConcludeFailedEpisode(ctx, tx, identity.EpisodeID)
	})
}

// markCancelling moves the attempt through AttemptCancelling, which the
// attempt lifecycle requires before AttemptCancelled.
func (r *Runner) markCancelling(ctx context.Context, tx *sql.Tx, identity Identity) error {
	var current AttemptStatus
	if err := tx.QueryRowContext(ctx, "SELECT status FROM episode_attempts WHERE attempt_id = ? AND episode_id = ? AND fence = ?", identity.AttemptID, identity.EpisodeID, identity.Fence).Scan(&current); err != nil {
		return fmt.Errorf("read episode attempt status: %w", err)
	}
	if current == AttemptCancelling {
		return nil
	}
	if err := TransitionAttempt(ctx, tx, identity, AttemptCancelling, r.clk.Now(), nil); err != nil {
		return fmt.Errorf("mark episode attempt cancelling: %w", err) //nolint:misspell // Durable lifecycle value is frozen as cancelling.
	}
	return nil
}

// retryOrConcludeFailedEpisode keeps a failed episode running for another
// attempt, or concludes it and settles its cost when the episode was
// superseded or has used maxEpisodeAttempts.
func (r *Runner) retryOrConcludeFailedEpisode(ctx context.Context, tx *sql.Tx, episodeID string) error {
	var lifecycle string
	if err := tx.QueryRowContext(ctx, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", episodeID).Scan(&lifecycle); err != nil {
		return fmt.Errorf("read episode lifecycle: %w", err)
	}
	failedAttempts, err := countFailedAttempts(ctx, tx, episodeID)
	if err != nil {
		return fmt.Errorf("count failed episode attempts: %w", err)
	}
	superseded := lifecycle == string(LifecycleSuperseded)
	if !superseded && failedAttempts < maxEpisodeAttempts {
		if _, err := tx.ExecContext(ctx, `
			UPDATE episodes SET lifecycle_status = 'running', ended_at = NULL, terminal_json = NULL
			WHERE episode_id = ?`,
			episodeID,
		); err != nil {
			return fmt.Errorf("update episode failed: %w", err)
		}
		return nil
	}
	if !superseded {
		terminalJSON, err := json.Marshal(map[string]any{"status": AttemptFailed, "reason": "attempt_retry_limit"})
		if err != nil {
			return fmt.Errorf("marshal retry limit terminal: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE episodes SET lifecycle_status = 'concluded', ended_at = ?, terminal_json = ? WHERE episode_id = ?", r.runtimeNow(), terminalJSON, episodeID); err != nil {
			return fmt.Errorf("conclude exhausted episode: %w", err)
		}
	}
	if r.cost != nil {
		if err := r.cost.Settle(ctx, tx, episodeID, 0, r.runtimeNow()); err != nil {
			return fmt.Errorf("settle abandoned episode cost: %w", err)
		}
	}
	return nil
}

func (r *Runner) failAttemptWithRejection(ctx context.Context, current, incoming Identity, reason RejectionReason, detail string) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		details, err := json.Marshal(map[string]any{"message": detail, "incoming_attempt_id": incoming.AttemptID, "incoming_fence": incoming.Fence})
		if err != nil {
			return fmt.Errorf("marshal identity rejection: %w", err)
		}
		if err := RecordRejection(ctx, tx, incoming, reason, details, r.clk.Now()); err != nil {
			return fmt.Errorf("record worker identity rejection: %w", err)
		}
		terminalJSON, err := json.Marshal(map[string]any{"status": AttemptFailed, "reason": detail})
		if err != nil {
			return fmt.Errorf("marshal terminal: %w", err)
		}
		if err := TransitionAttempt(ctx, tx, current, AttemptFailed, r.clk.Now(), terminalJSON); err != nil {
			return fmt.Errorf("finish identity-failed attempt: %w", err)
		}
		failedAttempts, err := countFailedAttempts(ctx, tx, current.EpisodeID)
		if err != nil {
			return fmt.Errorf("count identity-failed attempts: %w", err)
		}
		if failedAttempts < maxEpisodeAttempts {
			if _, err := tx.ExecContext(ctx, `
				UPDATE episodes SET lifecycle_status = 'running', ended_at = NULL, terminal_json = NULL
				WHERE episode_id = ?`, current.EpisodeID); err != nil {
				return fmt.Errorf("retain episode for retry: %w", err)
			}
			return nil
		}
		if r.cost != nil {
			if err := r.cost.Settle(ctx, tx, current.EpisodeID, 0, r.runtimeNow()); err != nil {
				return fmt.Errorf("settle exhausted episode cost: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE episodes SET lifecycle_status = 'concluded', ended_at = ?, terminal_json = ? WHERE episode_id = ?", r.runtimeNow(), terminalJSON, current.EpisodeID); err != nil {
			return fmt.Errorf("conclude identity-failed episode: %w", err)
		}
		return nil
	})
}

func countFailedAttempts(ctx context.Context, tx *sql.Tx, episodeID string) (int, error) {
	var failedAttempts int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM episode_attempts WHERE episode_id = ? AND status IN ('failed', 'timed_out', 'cancelled')`, episodeID).Scan(&failedAttempts); err != nil {
		return 0, fmt.Errorf("count failed attempts: %w", err)
	}
	return failedAttempts, nil
}
