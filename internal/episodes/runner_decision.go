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
	"time"
)

func decisionDigestForStorage(raw []byte) ([]byte, bool) {
	canonical, err := canonicaljson.Marshal(json.RawMessage(raw))
	if err != nil {
		return nil, false
	}
	var document map[string]any
	if err := json.Unmarshal(canonical, &document); err != nil {
		return nil, false
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, document)
	if err != nil {
		return nil, false
	}
	decoded, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		return nil, false
	}
	return decoded, true
}

func bindAttemptIdentity(raw []byte, identity Identity) ([]byte, error) {
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

func decisionInput(req *Request, identity Identity, now time.Time) (decisions.Input, error) {
	var payload struct {
		AllowedIntentTypes []string `json:"allowed_intent_types"`
		RiskCeiling        string   `json:"risk_ceiling"`
		Kind               string   `json:"kind"`
		Executor           struct {
			IntentCatalog       []map[string]any `json:"intent_catalog"`
			IntentCatalogSHA256 string           `json:"intent_catalog_sha256"`
		} `json:"executor"`
	}
	if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
		return decisions.Input{}, fmt.Errorf("decode request tools: %w", err)
	}
	allowed := make(map[string]struct{}, len(payload.AllowedIntentTypes))
	for _, intentType := range payload.AllowedIntentTypes {
		allowed[intentType] = struct{}{}
	}
	if payload.RiskCeiling == "" {
		return decisions.Input{}, fmt.Errorf("request has no explicit risk ceiling")
	}
	// P4/B10: the catalog is verified INDEPENDENTLY at the validation
	// boundary — the digest must bind the parsed bytes under the shared
	// domain; a forged/missing/empty catalog fails closed before the decision
	// is trusted (the worker's own verify_wire is not evidence here).
	if !canonicaljson.Verify(canonicaljson.DomainIntentCatalog, payload.Executor.IntentCatalog, payload.Executor.IntentCatalogSHA256) {
		return decisions.Input{}, fmt.Errorf("intent catalog is missing, forged, or malformed")
	}
	compiled, err := decisions.CompileIntentCatalog(payload.Executor.IntentCatalog)
	if err != nil {
		return decisions.Input{}, fmt.Errorf("compile intent catalog: %w", err)
	}
	return decisions.Input{
		EpisodeID:          identity.EpisodeID,
		AttemptID:          identity.AttemptID,
		Fence:              identity.Fence,
		TenantID:           req.TenantID,
		SituationID:        req.SituationID,
		SituationVersion:   req.SituationVersion,
		EntityID:           req.EntityID,
		SnapshotDigest:     req.SnapshotSHA256,
		AllowedIntentTypes: allowed,
		RiskCeiling:        payload.RiskCeiling,
		IntentCatalog:      compiled,
		Kind:               payload.Kind,
		Now:                now,
	}, nil
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func decisionIDFromJSON(raw []byte) string {
	var document struct {
		DecisionID string `json:"decision_id"`
	}
	if json.Unmarshal(raw, &document) != nil {
		return ""
	}
	return document.DecisionID
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
