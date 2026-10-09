package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

const deterministicBaselineVersion = "deterministic-baseline-v1"

// Intent is the baseline's projection of a catalog intent: its type, risk
// class and typed parameter schema. The application layer decodes the schema
// once at the boundary so this policy never asserts on raw maps.
type Intent struct {
	Type   string
	Risk   string
	Schema ParameterSchema
}

// ParameterSchema is the closed subset of a catalog intent's JSON Schema the
// baseline selects parameters from.
type ParameterSchema struct {
	Properties map[string]ParameterProperty
	Required   []any
}

// ParameterProperty is one declared parameter field.
type ParameterProperty struct {
	Type string
	Enum []any
}

// ParseParameterSchema decodes a spec intent's raw schema document once,
// tolerating absent or malformed parts exactly as the raw-map policy did.
func ParseParameterSchema(raw map[string]any) ParameterSchema {
	properties, _ := raw["properties"].(map[string]any)
	schema := ParameterSchema{Properties: make(map[string]ParameterProperty, len(properties))}
	for field, value := range properties {
		schema.Properties[field] = parseParameterProperty(value)
	}
	if required, ok := raw["required"].([]any); ok {
		schema.Required = required
	}
	return schema
}

func parseParameterProperty(value any) ParameterProperty {
	property, _ := value.(map[string]any)
	decoded := ParameterProperty{Enum: propertyEnum(property)}
	decoded.Type, _ = property["type"].(string)
	return decoded
}

func propertyEnum(property map[string]any) []any {
	enum, _ := property["enum"].([]any)
	return enum
}

// BaselinePolicy is the in-repository non-model shadow policy. It selects
// only from the declared intent catalog and emits no action plane object. Its
// simple rule is intentionally transparent: it selects the first declared
// intent and fills only schema-declared, bounded values; if the catalog cannot
// be satisfied it abstains explicitly.
type BaselinePolicy struct {
	intents []Intent
}

type baselineSnapshot struct {
	Phase  string `json:"phase"`
	Entity struct {
		ID string `json:"id"`
	} `json:"entity"`
}

// NewBaselinePolicy creates the baseline from an immutable intent catalog.
// The caller still validates its output through the normal Decision and
// Intent catalog validator.
func NewBaselinePolicy(intents []Intent) (*BaselinePolicy, error) {
	if len(intents) == 0 {
		return nil, fmt.Errorf("deterministic baseline requires a non-empty intent catalog")
	}
	return &BaselinePolicy{intents: append([]Intent(nil), intents...)}, nil
}

// ExecuteBaseline evaluates one immutable shadow snapshot and returns a
// deterministic Decision artifact. It has no access to credentials, workers,
// effectors, or storage.
func (b *BaselinePolicy) ExecuteBaseline(_ context.Context, input ShadowInput) (ShadowOutput, error) {
	if b == nil || len(b.intents) == 0 {
		return ShadowOutput{}, fmt.Errorf("deterministic baseline is not configured")
	}
	snapshot, err := decodeBaselineSnapshot(input.SnapshotJSON)
	if err != nil {
		return ShadowOutput{}, err
	}
	decision := newBaselineDecision(input)
	intent, err := b.selectIntent(input, snapshot)
	if err != nil {
		return ShadowOutput{}, err
	}
	setBaselineIntents(decision, intent)
	return finalizeBaselineOutput(input, decision)
}

func decodeBaselineSnapshot(raw []byte) (baselineSnapshot, error) {
	var snapshot baselineSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return baselineSnapshot{}, fmt.Errorf("decode baseline snapshot: %w", err)
	}
	return snapshot, nil
}

func newBaselineDecision(input ShadowInput) map[string]any {
	return map[string]any{
		"decision_id": "dec_baseline_" + shortKey(input.EpisodeKey), "episode_id": input.EpisodeID, "attempt_id": input.AttemptID,
		"fence": input.Fence, "snapshot_digest": input.SnapshotDigest,
		"situation_id": input.SituationID, "situation_version": input.SituationVersion,
		"confidence": 1.0,
	}
}

func (b *BaselinePolicy) selectIntent(input ShadowInput, snapshot baselineSnapshot) (map[string]any, error) {
	decisionID := "dec_baseline_" + shortKey(input.EpisodeKey)
	for _, configured := range b.intents {
		parameters, ok := baselineParameters(configured, snapshot.Entity.ID, snapshot.Phase)
		if !ok {
			continue
		}
		return newBaselineIntent(input, decisionID, configured, parameters)
	}
	return nil, nil
}

func newBaselineIntent(input ShadowInput, decisionID string, configured Intent, parameters map[string]any) (map[string]any, error) {
	intent := map[string]any{
		"intent_id":   "int_baseline_" + shortKey(input.EpisodeKey),
		"decision_id": decisionID, "tenant_id": input.TenantID,
		"situation_id": input.SituationID, "situation_version": input.SituationVersion,
		"type": configured.Type, "risk_class": configured.Risk,
		"parameters": parameters, "expires_at": "2099-01-01T00:00:00Z",
	}
	intentDigest, err := contractsv1.IntentDigest(intent)
	if err != nil {
		return nil, fmt.Errorf("digest baseline intent: %w", err)
	}
	intent["intent_digest"] = intentDigest
	return intent, nil
}

func setBaselineIntents(decision map[string]any, intent map[string]any) {
	if intent == nil {
		decision["decision_type"] = "need_more_evidence"
		decision["intents"] = []any{}
	} else {
		decision["intents"] = []any{intent}
	}
}

func finalizeBaselineOutput(input ShadowInput, decision map[string]any) (ShadowOutput, error) {
	decisionJSON, sum, err := canonicaljson.Seal(canonicaljson.DomainDecision, decision)
	if err != nil {
		return ShadowOutput{}, fmt.Errorf("seal baseline decision: %w", err)
	}
	decisionDigest := canonicaljson.EncodeDigest(sum)
	manifestDigest, err := baselineManifestDigest(input, decisionDigest)
	if err != nil {
		return ShadowOutput{}, err
	}
	return ShadowOutput{ExecutorVersion: deterministicBaselineVersion, ManifestSHA256: manifestDigest, DecisionJSON: decisionJSON, DecisionSHA256: decisionDigest}, nil
}

func baselineManifestDigest(input ShadowInput, decisionDigest string) (string, error) {
	manifestDigest, err := canonicaljson.Digest(canonicaljson.DomainShadowComparison, map[string]any{
		"executor_version": deterministicBaselineVersion, "episode_key": input.EpisodeKey,
		"decision_digest": decisionDigest,
	})
	if err != nil {
		return "", fmt.Errorf("digest baseline manifest: %w", err)
	}
	return manifestDigest, nil
}

func baselineParameters(configured Intent, entityID, phase string) (map[string]any, bool) {
	parameters := make(map[string]any)
	if _, ok := configured.Schema.Properties["entity_id"]; ok {
		if entityID == "" {
			return nil, false
		}
		parameters["entity_id"] = entityID
	}
	fillBaselineEnums(parameters, configured.Schema.Properties, phase)
	if !requiredBaselineParametersPresent(configured.Schema.Required, parameters) {
		return nil, false
	}
	return parameters, true
}

func fillBaselineEnums(parameters map[string]any, properties map[string]ParameterProperty, phase string) {
	for field, property := range properties {
		if field == "entity_id" {
			continue
		}
		if len(property.Enum) == 0 {
			continue
		}
		parameters[field] = baselineEnumValue(field, property.Enum, phase)
	}
}

func baselineEnumValue(field string, enum []any, phase string) any {
	value := enum[0]
	if field == "state" {
		value = enumValue(enum, map[string]string{"over_ceiling": "alert", "cooling": "watch"}, phase)
	}
	if field == "mode" {
		value = enumValue(enum, map[string]string{"over_ceiling": "bounded_cooling", "cooling": "hold"}, phase)
	}
	return value
}

func requiredBaselineParametersPresent(required []any, parameters map[string]any) bool {
	for _, raw := range required {
		field, ok := raw.(string)
		if !ok {
			return false
		}
		if _, present := parameters[field]; !present {
			return false
		}
	}
	return true
}

func enumValue(values []any, byPhase map[string]string, phase string) any {
	wanted, ok := byPhase[phase]
	if !ok {
		return values[0]
	}
	for _, value := range values {
		if value == wanted {
			return value
		}
	}
	return values[0]
}

func shortKey(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:8])
}
