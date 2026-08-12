// Package decisions validates worker-proposed Decisions at the stream boundary.
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
type Input struct {
	EpisodeID          string
	AttemptID          string
	Fence              int64
	TenantID           string
	SituationID        string
	SituationVersion   int
	SnapshotDigest     string
	AllowedIntentTypes map[string]struct{}
	RiskCeiling        string
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
	ID            string
	Type          string
	RiskClass     string
	ExpiresAt     time.Time
	Digest        string
	CanonicalJSON []byte
	Document      map[string]any
}

// ValidationError is a fail-closed Decision rejection. Reason values map to
// the durable lifecycle rejection registry.
type ValidationError struct {
	Reason  string
	Details map[string]any
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("decision rejected: %s", e.Reason)
}

// Validate parses, schema-validates, binds, and digests one worker Decision.
// It does not write to storage and never changes the attempt terminal state.
func Validate(raw []byte, transmittedDigest string, input Input) (*Result, error) {
	if input.Now.IsZero() {
		return nil, reject("schema_invalid", "clock", "trusted validation time is required")
	}
	canonical, err := canonicaljson.Marshal(json.RawMessage(raw))
	if err != nil {
		return nil, reject("schema_invalid", "canonical_json", err.Error())
	}
	var document map[string]any
	if err := json.Unmarshal(canonical, &document); err != nil {
		return nil, reject("schema_invalid", "json", err.Error())
	}
	if err := contractsv1.Validate(contractsv1.SchemaDecision, document); err != nil {
		return nil, reject("schema_invalid", "decision_schema", err.Error())
	}
	if _, err := canonicaljson.DecodeDigest(transmittedDigest); err != nil || !canonicaljson.Verify(canonicaljson.DomainDecision, document, transmittedDigest) {
		return nil, reject("schema_invalid", "decision_digest", "decision digest is missing or does not match canonical JSON")
	}

	decisionID, ok := document["decision_id"].(string)
	if !ok || decisionID == "" {
		return nil, reject("schema_invalid", "decision_id", "decision_id is required")
	}
	if got, _ := document["episode_id"].(string); got != input.EpisodeID {
		return nil, reject("snapshot_mismatch", "episode_id", "decision episode does not match the dispatched episode")
	}
	if got, _ := document["attempt_id"].(string); got != input.AttemptID {
		return nil, reject("stale_attempt", "attempt_id", "decision attempt does not match the dispatched attempt")
	}
	if got, ok := integerField(document, "fence"); !ok || int64(got) != input.Fence {
		return nil, reject("stale_attempt", "fence", "decision fence does not match the dispatched attempt")
	}
	if got, _ := document["snapshot_digest"].(string); got != input.SnapshotDigest {
		return nil, reject("snapshot_mismatch", "snapshot_digest", "decision snapshot does not match the dispatched snapshot")
	}
	if got, _ := document["situation_id"].(string); got != input.SituationID {
		return nil, reject("snapshot_mismatch", "situation_id", "decision Situation does not match the dispatched Situation")
	}
	if got, ok := integerField(document, "situation_version"); !ok || got != input.SituationVersion {
		return nil, reject("snapshot_mismatch", "situation_version", "decision Situation version does not match the dispatched version")
	}
	if validUntil, ok := document["valid_until"].(string); ok && isExpired(input.Now, validUntil) {
		return nil, reject("expired", "valid_until", "decision validity has expired")
	}

	rawIntents, ok := document["intents"].([]any)
	if !ok {
		return nil, reject("schema_invalid", "intents", "intents must be an array")
	}
	result := &Result{
		DecisionID:     decisionID,
		DecisionDigest: transmittedDigest,
		CanonicalJSON:  canonical,
		Document:       document,
		Intents:        make([]Intent, 0, len(rawIntents)),
	}
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
		validated, err := validateIntent(intent, input, decisionID, seenIDs)
		if err != nil {
			return nil, err
		}
		result.Intents = append(result.Intents, *validated)
	}
	return result, nil
}

func validateIntent(document map[string]any, input Input, decisionID string, seenIDs map[string]struct{}) (*Intent, error) {
	intentID, _ := document["intent_id"].(string)
	if _, exists := seenIDs[intentID]; exists {
		return nil, reject("schema_invalid", "intent_id", "Decision contains duplicate intent_id values")
	}
	seenIDs[intentID] = struct{}{}
	if got, _ := document["decision_id"].(string); got != decisionID {
		return nil, reject("snapshot_mismatch", "intent.decision_id", "intent is not bound to its Decision")
	}
	if got, _ := document["tenant_id"].(string); got != input.TenantID {
		return nil, reject("snapshot_mismatch", "intent.tenant_id", "intent tenant does not match the episode")
	}
	if got, _ := document["situation_id"].(string); got != input.SituationID {
		return nil, reject("snapshot_mismatch", "intent.situation_id", "intent Situation does not match the episode")
	}
	if got, ok := integerField(document, "situation_version"); !ok || got != input.SituationVersion {
		return nil, reject("snapshot_mismatch", "intent.situation_version", "intent Situation version does not match the episode")
	}
	intentType, _ := document["type"].(string)
	if _, allowed := input.AllowedIntentTypes[intentType]; !allowed {
		return nil, reject("intent_type_not_allowed", "intent.type", "intent type is not allowed for this episode")
	}
	risk, _ := document["risk_class"].(string)
	if riskRank(risk) == 0 || riskRank(risk) > riskRank(input.RiskCeiling) {
		return nil, reject("risk_ceiling_exceeded", "intent.risk_class", "intent risk exceeds the episode ceiling")
	}
	expiresAtString, _ := document["expires_at"].(string)
	expiresAt, err := time.Parse(time.RFC3339Nano, expiresAtString)
	if err != nil {
		return nil, reject("schema_invalid", "intent.expires_at", err.Error())
	}
	if !expiresAt.After(input.Now) {
		return nil, reject("expired", "intent.expires_at", "intent has expired")
	}
	canonical, err := canonicaljson.Marshal(document)
	if err != nil {
		return nil, reject("schema_invalid", "intent", err.Error())
	}
	digest, err := contractsv1.IntentDigest(document)
	if err != nil {
		return nil, reject("schema_invalid", "intent_digest", err.Error())
	}
	return &Intent{
		ID:            intentID,
		Type:          intentType,
		RiskClass:     risk,
		ExpiresAt:     expiresAt,
		Digest:        digest,
		CanonicalJSON: canonical,
		Document:      document,
	}, nil
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

func isExpired(now time.Time, value string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && !now.Before(parsed)
}
