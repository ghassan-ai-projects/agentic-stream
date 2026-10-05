package episodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/domain"
	"time"

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

type decisionRequestAuthority struct {
	AllowedIntentTypes []string `json:"allowed_intent_types"`
	RiskCeiling        string   `json:"risk_ceiling"`
	Kind               string   `json:"kind"`
	Executor           struct {
		IntentCatalog       []map[string]any `json:"intent_catalog"`
		IntentCatalogSHA256 string           `json:"intent_catalog_sha256"`
	} `json:"executor"`
}

const insertDecisionSQL = `
		INSERT INTO decisions (
			decision_id, episode_id, attempt_id, fence, ordinal, situation_id,
			situation_version, raw_json, decision_sha256, validation_status,
			validation_json, traceparent, tracestate, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

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
	record.id = domain.DecisionIDFromJSON(outcome.DecisionJSON)
	if record.id == "" {
		record.id = r.idGen.New(ids.PrefixDecision)
	}
	digest, hasContractDigest := domain.StorageDecisionDigest(outcome.DecisionJSON)
	return record.finishValidation(outcome.DecisionJSON, digest, hasContractDigest)
}

func decisionInput(req *Request, identity episodeledger.Identity, now time.Time) (decisions.Input, error) {
	var payload decisionRequestAuthority
	if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
		return decisions.Input{}, fmt.Errorf("decode request tools: %w", err)
	}
	return payload.validationInput(req, identity, now)
}

func (payload decisionRequestAuthority) validationInput(req *Request, identity episodeledger.Identity, now time.Time) (decisions.Input, error) {
	allowed := make(map[string]struct{}, len(payload.AllowedIntentTypes))
	for _, intentType := range payload.AllowedIntentTypes {
		allowed[intentType] = struct{}{}
	}
	if payload.RiskCeiling == "" {
		return decisions.Input{}, fmt.Errorf("request has no explicit risk ceiling")
	}
	compiled, err := payload.verifiedIntentCatalog()
	if err != nil {
		return decisions.Input{}, err
	}
	return payload.boundValidationInput(req, identity, now, allowed, compiled), nil
}

func (payload decisionRequestAuthority) verifiedIntentCatalog() (*decisions.IntentCatalog, error) {
	if !canonicaljson.Verify(canonicaljson.DomainIntentCatalog, payload.Executor.IntentCatalog, payload.Executor.IntentCatalogSHA256) {
		return nil, fmt.Errorf("intent catalog is missing, forged, or malformed")
	}
	compiled, err := decisions.CompileIntentCatalog(payload.Executor.IntentCatalog)
	if err != nil {
		return nil, fmt.Errorf("compile intent catalog: %w", err)
	}
	return compiled, nil
}

func (payload decisionRequestAuthority) boundValidationInput(req *Request, identity episodeledger.Identity, now time.Time, allowed map[string]struct{}, compiled *decisions.IntentCatalog) decisions.Input {
	return decisions.Input{
		EpisodeID: identity.EpisodeID, AttemptID: identity.AttemptID, Fence: identity.Fence,
		TenantID: req.TenantID, SituationID: req.SituationID, SituationVersion: req.SituationVersion,
		EntityID: req.EntityID, SnapshotDigest: req.SnapshotSHA256,
		AllowedIntentTypes: allowed, RiskCeiling: payload.RiskCeiling, IntentCatalog: compiled,
		Kind: payload.Kind, Now: now,
	}
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

func insertDecision(ctx context.Context, tx *sql.Tx, claim *episodeClaim, outcome *Outcome, record *decisionRecord, now string) error {
	validationStatus := "rejected"
	if record.validationErr == nil {
		validationStatus = "proposed"
	}
	var ordinal int
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(ordinal), 0) + 1 FROM decisions WHERE episode_id = ?", claim.episodeID).Scan(&ordinal); err != nil {
		return fmt.Errorf("allocate decision ordinal: %w", err)
	}
	return insertDecisionRow(ctx, tx, claim, outcome, record, now, ordinal, validationStatus)
}

func insertDecisionRow(ctx context.Context, tx *sql.Tx, claim *episodeClaim, outcome *Outcome, record *decisionRecord, now string, ordinal int, validationStatus string) error {
	if _, err := tx.ExecContext(ctx, insertDecisionSQL,
		record.id, claim.episodeID, claim.identity.AttemptID, claim.identity.Fence, ordinal,
		claim.req.SituationID, claim.req.SituationVersion,
		outcome.DecisionJSON, record.digest, validationStatus, record.validationJSON,
		nullableString(claim.req.Traceparent), nullableString(claim.req.Tracestate), now,
	); err != nil {
		return fmt.Errorf("insert decision: %w", err)
	}
	return nil
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func (r *Runner) rejectDecision(ctx context.Context, tx *sql.Tx, identity episodeledger.Identity, record *decisionRecord) error {
	reason := "schema_invalid"
	var typed *decisions.ValidationError
	if errors.As(record.validationErr, &typed) {
		reason = typed.Reason
	}
	if err := episodeledger.RecordRejection(ctx, tx, identity, episodeledger.RejectionReason(reason), record.validationJSON, r.clk.Now()); err != nil {
		return fmt.Errorf("record decision rejection: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE decisions SET rejection_reason = ? WHERE decision_id = ?", reason, record.id); err != nil {
		return fmt.Errorf("annotate rejected decision: %w", err)
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

func bindAttemptIdentity(raw []byte, identity episodeledger.Identity) ([]byte, error) {
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode request json: %w", err)
	}
	document["attempt_id"] = identity.AttemptID
	document["fence"] = identity.Fence
	bound, err := canonicaljson.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("canonicalize request json: %w", err)
	}
	return bound, nil
}
