package replay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const deterministicBaselineVersion = "deterministic-baseline-v1"

// DeterministicBaseline is the in-repository non-model shadow policy. It
// selects only from the spec's declared intent catalog and emits no action
// plane object. Its simple rule is intentionally transparent: it selects the
// first declared intent and fills only schema-declared, bounded values; if the
// catalog cannot be satisfied it abstains explicitly.
type DeterministicBaseline struct {
	intents []spec.Intent
}

// NewDeterministicBaseline creates the baseline from an immutable compiled
// spec. The caller still validates its output through the normal Decision and
// Intent catalog validator.
func NewDeterministicBaseline(compiled *spec.CompiledSpec) (*DeterministicBaseline, error) {
	if compiled == nil || len(compiled.Actions.Intents) == 0 {
		return nil, fmt.Errorf("deterministic baseline requires a non-empty intent catalog")
	}
	intents := append([]spec.Intent(nil), compiled.Actions.Intents...)
	return &DeterministicBaseline{intents: intents}, nil
}

// ExecuteBaseline evaluates one immutable shadow snapshot and returns a
// deterministic Decision artifact. It has no access to credentials, workers,
// effectors, or storage.
func (b *DeterministicBaseline) ExecuteBaseline(_ context.Context, input ShadowInput) (ShadowOutput, error) {
	if b == nil || len(b.intents) == 0 {
		return ShadowOutput{}, fmt.Errorf("deterministic baseline is not configured")
	}
	var snapshot struct {
		Phase  string `json:"phase"`
		Entity struct {
			ID string `json:"id"`
		} `json:"entity"`
	}
	if err := json.Unmarshal(input.SnapshotJSON, &snapshot); err != nil {
		return ShadowOutput{}, fmt.Errorf("decode baseline snapshot: %w", err)
	}
	decisionID := "dec_baseline_" + shortKey(input.EpisodeKey)
	decision := map[string]any{
		"decision_id": decisionID, "episode_id": input.EpisodeID, "attempt_id": input.AttemptID,
		"fence": input.Fence, "snapshot_digest": input.SnapshotDigest,
		"situation_id": input.SituationID, "situation_version": input.SituationVersion,
		"confidence": 1.0,
	}
	for _, configured := range b.intents {
		parameters, ok := baselineParameters(configured, snapshot.Entity.ID, snapshot.Phase)
		if !ok {
			continue
		}
		intent := map[string]any{
			"intent_id":   "int_baseline_" + shortKey(input.EpisodeKey),
			"decision_id": decisionID, "tenant_id": input.TenantID,
			"situation_id": input.SituationID, "situation_version": input.SituationVersion,
			"type": configured.Type, "risk_class": configured.Risk,
			"parameters": parameters, "expires_at": "2099-01-01T00:00:00Z",
		}
		intentDigest, err := contractsv1.IntentDigest(intent)
		if err != nil {
			return ShadowOutput{}, fmt.Errorf("digest baseline intent: %w", err)
		}
		intent["intent_digest"] = intentDigest
		decision["intents"] = []any{intent}
		break
	}
	if _, ok := decision["intents"]; !ok {
		decision["decision_type"] = "need_more_evidence"
		decision["intents"] = []any{}
	}
	decisionJSON, err := canonicaljson.Marshal(decision)
	if err != nil {
		return ShadowOutput{}, fmt.Errorf("marshal baseline decision: %w", err)
	}
	decisionDigest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		return ShadowOutput{}, fmt.Errorf("digest baseline decision: %w", err)
	}
	manifestDigest, err := canonicaljson.Digest(canonicaljson.DomainShadowComparison, map[string]any{
		"executor_version": deterministicBaselineVersion, "episode_key": input.EpisodeKey,
		"decision_digest": decisionDigest,
	})
	if err != nil {
		return ShadowOutput{}, fmt.Errorf("digest baseline manifest: %w", err)
	}
	return ShadowOutput{ExecutorVersion: deterministicBaselineVersion, ManifestSHA256: manifestDigest, DecisionJSON: decisionJSON, DecisionSHA256: decisionDigest}, nil
}

func baselineParameters(configured spec.Intent, entityID, phase string) (map[string]any, bool) {
	properties, _ := configured.ParameterSchema["properties"].(map[string]any)
	parameters := make(map[string]any)
	if _, ok := properties["entity_id"]; ok {
		if entityID == "" {
			return nil, false
		}
		parameters["entity_id"] = entityID
	}
	for field, raw := range properties {
		if field == "entity_id" {
			continue
		}
		property, _ := raw.(map[string]any)
		enum, _ := property["enum"].([]any)
		if len(enum) == 0 {
			continue
		}
		value := enum[0]
		if field == "state" {
			value = enumValue(enum, map[string]string{"over_ceiling": "alert", "cooling": "watch"}, phase)
		}
		if field == "mode" {
			value = enumValue(enum, map[string]string{"over_ceiling": "bounded_cooling", "cooling": "hold"}, phase)
		}
		parameters[field] = value
	}
	required, _ := configured.ParameterSchema["required"].([]any)
	for _, raw := range required {
		field, ok := raw.(string)
		if !ok {
			return nil, false
		}
		if _, present := parameters[field]; !present {
			return nil, false
		}
	}
	return parameters, true
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
