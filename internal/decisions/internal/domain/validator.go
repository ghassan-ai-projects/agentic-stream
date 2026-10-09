package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

// Input is the immutable stream context against which a worker Decision is
// validated. The worker-provided document never supplies these values.
// P4: IntentCatalog is the compiled, digest-verified catalog — the domain's
// authority (declared risks, parameter schemas, presets, model-writable
// fields). AllowedIntentTypes remains the per-episode subset.
type Input struct {
	EpisodeID          string
	AttemptID          string
	Fence              int64
	TenantID           string
	SituationID        string
	SituationVersion   int
	EntityID           string
	SnapshotDigest     string
	AllowedIntentTypes map[string]struct{}
	RiskCeiling        string
	IntentCatalog      *IntentCatalog
	Reconsider         bool
	Now                time.Time
}

// Result is a validated Decision and its independently digested Intents.
type Result struct {
	DecisionID     string
	DecisionDigest string
	Intents        []Intent
}

// Intent is one validated child of a Decision.
type Intent struct {
	ID               string
	Type             string
	RiskClass        string
	ExpiresAt        time.Time
	Digest           string
	CanonicalJSON    []byte
	RateLimitPerHour int
	RequiresApproval bool
}

// ValidationError is a fail-closed Decision rejection. Reason values map to
// the durable lifecycle rejection registry.
type ValidationError struct {
	Reason  string
	Details map[string]any
}

func (e *ValidationError) Error() string {
	if e.Details != nil {
		if field, ok := e.Details["field"].(string); ok {
			return fmt.Sprintf("decision rejected: %s (%s)", e.Reason, field)
		}
	}
	return fmt.Sprintf("decision rejected: %s", e.Reason)
}

// Validate parses, schema-validates, binds, and digests one worker Decision.
// It does not write to storage and never changes the attempt terminal state.
func Validate(raw []byte, transmittedDigest string, input Input) (*Result, error) {
	if err := checkTrustedInput(input); err != nil {
		return nil, err
	}
	document, err := parseDecision(raw, transmittedDigest)
	if err != nil {
		return nil, err
	}
	decisionID, err := checkDecisionBinding(document, input)
	if err != nil {
		return nil, err
	}
	return acceptDecision(document, transmittedDigest, decisionID, input)
}

func validateDecisionIntents(rawIntents []any, input Input, decisionID string, document map[string]any) ([]Intent, error) {
	intents := make([]Intent, 0, len(rawIntents))
	seenIDs := make(map[string]struct{}, len(rawIntents))
	for index, rawIntent := range rawIntents {
		intent, err := decodeDecisionIntent(rawIntent, index)
		if err != nil {
			return nil, err
		}
		validated, err := validateIntent(intent, input, decisionID, document, seenIDs)
		if err != nil {
			return nil, err
		}
		intents = append(intents, *validated)
	}
	return intents, nil
}

func parseDecision(raw []byte, transmittedDigest string) (map[string]any, error) {
	document, err := contractsv1.DecodeDocument(raw, contractsv1.SchemaDecision)
	if err != nil {
		return nil, reject("schema_invalid", decodeFailureField(err), err.Error())
	}
	sum, err := canonicaljson.DecodeDigest(transmittedDigest)
	if err != nil || !contractsv1.VerifyDocumentDigest(canonicaljson.DomainDecision, document, sum) {
		return nil, reject("schema_invalid", "decision_digest", "decision digest is missing or does not match canonical JSON")
	}
	return document, nil
}

func decodeFailureField(err error) string {
	if errors.Is(err, contractsv1.ErrDocumentSchema) {
		return "decision_schema"
	}
	return "canonical_json"
}

func integerField(document map[string]any, name string) (int, bool) {
	value, ok := document[name].(float64)
	if !ok || value != float64(int(value)) {
		return 0, false
	}
	return int(value), true
}

func reject(reason, field, message string) *ValidationError {
	return &ValidationError{Reason: reason, Details: map[string]any{
		"field": field, "message": message,
	}}
}

// isExpired reports whether a decision's valid_until has passed. It fails
// closed: an unparseable trusted-side timestamp is treated as expired so the
// decision is rejected rather than admitted on a malformed validity window.
func isExpired(now time.Time, value string) bool {
	validUntil, err := kernel.ParseTime(value)
	return err != nil || !validUntil.After(now)
}

func checkTrustedInput(input Input) error {
	if input.Now.IsZero() {
		return reject("schema_invalid", "clock", "trusted validation time is required")
	}
	if input.IntentCatalog == nil {
		return reject("catalog_missing", "catalog", "the compiled intent catalog is required")
	}
	return nil
}

func acceptDecision(document map[string]any, transmittedDigest, decisionID string, input Input) (*Result, error) {
	rawIntents, err := decisionIntents(document)
	if err != nil {
		return nil, err
	}
	result := &Result{
		DecisionID:     decisionID,
		DecisionDigest: transmittedDigest,
	}
	result.Intents, err = validateDecisionIntents(rawIntents, input, decisionID, document)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func decodeDecisionIntent(rawIntent any, index int) (map[string]any, error) {
	intent, ok := rawIntent.(map[string]any)
	if !ok {
		return nil, reject("schema_invalid", fmt.Sprintf("intents[%d]", index), "intent must be an object")
	}
	if err := contractsv1.Validate(contractsv1.SchemaIntent, intent); err != nil {
		return nil, reject("schema_invalid", fmt.Sprintf("intents[%d]", index), err.Error())
	}
	if !contractsv1.VerifyIntentDigest(intent) {
		return nil, reject("schema_invalid", fmt.Sprintf("intents[%d].intent_digest", index), "intent digest does not match the canonical intent")
	}
	return intent, nil
}
