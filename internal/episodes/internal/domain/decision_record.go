package domain

import (
	"errors"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/decisions"
)

// DecisionRecord is a validated proposal and its durable storage evidence.
type DecisionRecord struct {
	ID             string
	Digest         []byte
	Validated      *decisions.Result
	ValidationErr  error
	ValidationJSON []byte
}

// ValidateDecision binds validation and the declared identity without allocating one.
func ValidateDecision(outcome *Outcome, input decisions.Input) *DecisionRecord {
	validated, err := decisions.Validate(outcome.DecisionJSON, outcome.DecisionSHA256, input)
	return &DecisionRecord{ID: DecisionIDFromJSON(outcome.DecisionJSON), Validated: validated, ValidationErr: err}
}

// PrepareStorage derives the canonical digest or traceable raw-hash failure evidence.
func (record *DecisionRecord) PrepareStorage(raw []byte) (*DecisionRecord, error) {
	digest, hasContractDigest := StorageDecisionDigest(raw)
	return record.finishValidation(raw, digest, hasContractDigest)
}
func (record *DecisionRecord) finishValidation(raw []byte, digest []byte, hasContractDigest bool) (*DecisionRecord, error) {
	var err error
	record.Digest = digest
	if record.ValidationErr == nil {
		record.ValidationJSON = []byte(`{}`)
		if decoded, decodeErr := canonicaljson.DecodeDigest(record.Validated.DecisionDigest); decodeErr == nil {
			record.Digest = decoded
		}
		return record, nil
	}
	record.ValidationJSON, err = ValidationFailureJSON(record.ValidationErr, raw, hasContractDigest)
	if err != nil {
		return nil, err
	}
	return record, nil
}

// ValidationStatus records a rejected or proposed decision before governance.
func (record *DecisionRecord) ValidationStatus() string {
	if record.ValidationErr == nil {
		return "proposed"
	}
	return "rejected"
}

// RejectionReason preserves the validator's typed reason or the schema fallback.
func (record *DecisionRecord) RejectionReason() string {
	var typed *decisions.ValidationError
	if errors.As(record.ValidationErr, &typed) {
		return typed.Reason
	}
	return "schema_invalid"
}
