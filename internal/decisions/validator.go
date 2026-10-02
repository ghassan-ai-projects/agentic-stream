package decisions

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
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
	Kind               string
	Now                time.Time
}

// Result is a validated Decision and its independently digested Intents.
type Result struct {
	DecisionID     string
	DecisionDigest string
	CanonicalJSON  []byte
	Document       map[string]any
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
	Document         map[string]any
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
	if input.Now.IsZero() {
		return nil, reject("schema_invalid", "clock", "trusted validation time is required")
	}
	if input.IntentCatalog == nil {
		return nil, reject("catalog_missing", "catalog", "the compiled intent catalog is required")
	}
	canonical, document, err := parseDecision(raw, transmittedDigest)
	if err != nil {
		return nil, err
	}
	decisionID, err := checkDecisionBinding(document, input)
	if err != nil {
		return nil, err
	}
	rawIntents, err := decisionIntents(document)
	if err != nil {
		return nil, err
	}
	result := &Result{
		DecisionID:     decisionID,
		DecisionDigest: transmittedDigest,
		CanonicalJSON:  canonical,
		Document:       document,
	}
	result.Intents, err = validateDecisionIntents(rawIntents, input, decisionID, document)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func validateDecisionIntents(rawIntents []any, input Input, decisionID string, document map[string]any) ([]Intent, error) {
	intents := make([]Intent, 0, len(rawIntents))
	seenIDs := make(map[string]struct{}, len(rawIntents))
	for index, rawIntent := range rawIntents {
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
		validated, err := validateIntent(intent, input, decisionID, document, seenIDs)
		if err != nil {
			return nil, err
		}
		intents = append(intents, *validated)
	}
	return intents, nil
}

// parseDecision canonicalizes the raw Decision, validates it against the
// shared schema, and verifies the transmitted digest.
func parseDecision(raw []byte, transmittedDigest string) ([]byte, map[string]any, error) {
	canonical, err := canonicaljson.Marshal(json.RawMessage(raw))
	if err != nil {
		return nil, nil, reject("schema_invalid", "canonical_json", err.Error())
	}
	var document map[string]any
	if err := json.Unmarshal(canonical, &document); err != nil {
		return nil, nil, reject("schema_invalid", "json", err.Error())
	}
	if err := contractsv1.Validate(contractsv1.SchemaDecision, document); err != nil {
		return nil, nil, reject("schema_invalid", "decision_schema", err.Error())
	}
	if _, err := canonicaljson.DecodeDigest(transmittedDigest); err != nil || !canonicaljson.Verify(canonicaljson.DomainDecision, document, transmittedDigest) {
		return nil, nil, reject("schema_invalid", "decision_digest", "decision digest is missing or does not match canonical JSON")
	}
	return canonical, document, nil
}

// checkDecisionBinding requires the Decision to name the dispatched episode,
// attempt, fence, snapshot, and Situation version, and to be unexpired. It
// returns the decision ID.
func checkDecisionBinding(document map[string]any, input Input) (string, error) {
	decisionID, ok := document["decision_id"].(string)
	if !ok || decisionID == "" {
		return "", reject("schema_invalid", "decision_id", "decision_id is required")
	}
	if got, _ := document["episode_id"].(string); got != input.EpisodeID {
		return "", reject("snapshot_mismatch", "episode_id", "decision episode does not match the dispatched episode")
	}
	if got, _ := document["attempt_id"].(string); got != input.AttemptID {
		return "", reject("stale_attempt", "attempt_id", "decision attempt does not match the dispatched attempt")
	}
	if got, ok := integerField(document, "fence"); !ok || int64(got) != input.Fence {
		return "", reject("stale_attempt", "fence", "decision fence does not match the dispatched attempt")
	}
	if got, _ := document["snapshot_digest"].(string); got != input.SnapshotDigest {
		return "", reject("snapshot_mismatch", "snapshot_digest", "decision snapshot does not match the dispatched snapshot")
	}
	if got, _ := document["situation_id"].(string); got != input.SituationID {
		return "", reject("snapshot_mismatch", "situation_id", "decision Situation does not match the dispatched Situation")
	}
	if got, ok := integerField(document, "situation_version"); !ok || got != input.SituationVersion {
		return "", reject("snapshot_mismatch", "situation_version", "decision Situation version does not match the dispatched version")
	}
	if validUntil, ok := document["valid_until"].(string); ok && isExpired(input.Now, validUntil) {
		return "", reject("expired", "valid_until", "decision validity has expired")
	}
	return decisionID, nil
}

// decisionIntents returns the intents array. An empty array must explicitly
// request more evidence, and (P4) at most ONE intent may be actionable
// (neither a watch nor a compensation); the v1 contract is enforced here
// independently of the worker's builder.
func decisionIntents(document map[string]any) ([]any, error) {
	rawIntents, ok := document["intents"].([]any)
	if !ok {
		return nil, reject("schema_invalid", "intents", "intents must be an array")
	}
	if len(rawIntents) == 0 {
		if decisionType, _ := document["decision_type"].(string); decisionType != "need_more_evidence" {
			return nil, reject("schema_invalid", "decision_type", "an empty-intent decision must explicitly request more evidence")
		}
	}
	actionable := 0
	for _, rawIntent := range rawIntents {
		intent, ok := rawIntent.(map[string]any)
		if !ok {
			return nil, reject("schema_invalid", "intents", "intent must be an object")
		}
		intentType, _ := intent["type"].(string)
		if intentType == "install_watch_condition" || documentString(intent, "compensates") != "" {
			continue
		}
		actionable++
	}
	if actionable > 1 {
		return nil, reject("schema_invalid", "intents",
			"a decision may carry at most one actionable intent")
	}
	return rawIntents, nil
}

func documentString(document map[string]any, key string) string {
	value, _ := document[key].(string)
	return value
}

func integerField(document map[string]any, name string) (int, bool) {
	value, ok := document[name].(float64)
	if !ok || value != float64(int(value)) {
		return 0, false
	}
	return int(value), true
}

func riskRank(risk string) int {
	switch risk {
	case "R0":
		return 1
	case "R1":
		return 2
	case "R2":
		return 3
	case "R3":
		return 4
	case "R4":
		return 5
	default:
		return 0
	}
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
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return true
	}
	return !now.Before(parsed)
}
