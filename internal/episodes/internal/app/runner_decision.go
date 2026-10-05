package app

import (
	"context"
	"errors"
	"fmt"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
)

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
func (r *Runner) persistDecision(ctx context.Context, tx *store.Tx, claim *episodeClaim, outcome *Outcome, now string) (*decisionRecord, error) {
	if outcome.DecisionJSON == nil {
		return nil, nil
	}
	record, err := r.validateDecision(claim, outcome)
	if err != nil {
		return nil, err
	}
	if err := store.InsertDecision(ctx, tx, decisionInsert(claim, outcome, record, now)); err != nil {
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
	validationInput, err := domain.DecisionInput(&claim.req, claim.identity, r.clk.Now())
	if err != nil {
		return nil, fmt.Errorf("build decision validation input: %w", err)
	}
	record := &decisionRecord{}
	record.validated, record.validationErr = decisions.Validate(outcome.DecisionJSON, outcome.DecisionSHA256, validationInput)
	record.id = domain.DecisionIDFromJSON(outcome.DecisionJSON)
	if record.id == "" {
		record.id = r.idGen.New(ids.PrefixDecision)
	}
	digest, hasContractDigest := domain.StorageDecisionDigest(outcome.DecisionJSON)
	return record.finishValidation(outcome.DecisionJSON, digest, hasContractDigest)
}

func (record *decisionRecord) finishValidation(raw []byte, digest []byte, hasContractDigest bool) (*decisionRecord, error) {
	var err error
	record.digest = digest
	if record.validationErr == nil {
		record.validationJSON = []byte(`{}`)
		if decoded, decodeErr := canonicaljson.DecodeDigest(record.validated.DecisionDigest); decodeErr == nil {
			record.digest = decoded
		}
		return record, nil
	}
	record.validationJSON, err = domain.ValidationFailureJSON(record.validationErr, raw, hasContractDigest)
	if err != nil {
		return nil, err
	}
	return record, nil
}

// decisionInsert binds a validated decision record to its episode attempt.
func decisionInsert(claim *episodeClaim, outcome *Outcome, record *decisionRecord, now string) store.DecisionInsert {
	validationStatus := "rejected"
	if record.validationErr == nil {
		validationStatus = "proposed"
	}
	return store.DecisionInsert{
		DecisionID: record.id, EpisodeID: claim.episodeID, AttemptID: claim.identity.AttemptID, Fence: claim.identity.Fence,
		SituationID: claim.req.SituationID, SituationVersion: claim.req.SituationVersion,
		RawJSON: outcome.DecisionJSON, Digest: record.digest, ValidationStatus: validationStatus,
		ValidationJSON: record.validationJSON, Traceparent: claim.req.Traceparent, Tracestate: claim.req.Tracestate, Now: now,
	}
}

func (r *Runner) rejectDecision(ctx context.Context, tx *store.Tx, identity episodeledger.Identity, record *decisionRecord) error {
	reason := "schema_invalid"
	var typed *decisions.ValidationError
	if errors.As(record.validationErr, &typed) {
		reason = typed.Reason
	}
	if err := tx.RecordRejection(ctx, identity, episodeledger.RejectionReason(reason), record.validationJSON, r.clk.Now()); err != nil {
		return fmt.Errorf("record decision rejection: %w", err)
	}
	if err := store.AnnotateRejectedDecision(ctx, tx, record.id, reason); err != nil {
		return err
	}
	return nil
}

// governDecision hands a valid Decision to the action plane, or only scores it
// in shadow mode, and marks it accepted.
func (r *Runner) governDecision(ctx context.Context, tx *store.Tx, claim *episodeClaim, outcome *Outcome, record *decisionRecord, now string) error {
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
	if err := store.AcceptDecision(ctx, tx, record.id); err != nil {
		return err
	}
	return nil
}
