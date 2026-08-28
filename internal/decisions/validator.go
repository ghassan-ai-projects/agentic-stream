package decisions

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/santhosh-tekuri/jsonschema/v6"
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

// IntentCatalog is the compiled catalog the validator enforces independently.
type IntentCatalog struct {
	Entries map[string]*IntentEntry
}

// IntentEntry is one declared action type's authority.
type IntentEntry struct {
	Type             string
	RiskClass        string
	ParameterSchema  *jsonschema.Schema
	Presets          map[string]map[string]any
	ModelWritable    map[string]bool
	RateLimitPerHour int
	RequiresApproval bool
}

// CompileIntentCatalog builds the fail-closed validator view from the wire
// document (already digest-verified by the caller): a missing, empty,
// duplicate, or structurally invalid catalog is an error — never repaired.
func CompileIntentCatalog(doc []map[string]any) (*IntentCatalog, error) {
	if len(doc) == 0 {
		return nil, fmt.Errorf("intent catalog is empty")
	}
	catalog := &IntentCatalog{Entries: make(map[string]*IntentEntry, len(doc))}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	compiler.UseLoader(denyNetworkLoader{})
	for _, entry := range doc {
		entryType, _ := entry["type"].(string)
		if entryType == "" {
			return nil, fmt.Errorf("intent catalog entry has no type")
		}
		if _, exists := catalog.Entries[entryType]; exists {
			return nil, fmt.Errorf("intent catalog duplicates type %q", entryType)
		}
		risk, _ := entry["risk_class"].(string)
		if riskRank(risk) == 0 {
			return nil, fmt.Errorf("intent %q has invalid declared risk %q", entryType, risk)
		}
		schema, ok := entry["parameter_schema"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("intent %q has no parameter schema", entryType)
		}
		schemaID := "urn:situation-runtime:catalog:" + entryType + ":schema:v1"
		if err := compiler.AddResource(schemaID, schema); err != nil {
			return nil, fmt.Errorf("compile intent %q schema: %w", entryType, err)
		}
		compiled, err := compiler.Compile(schemaID)
		if err != nil {
			return nil, fmt.Errorf("compile intent %q schema: %w", entryType, err)
		}
		writable := make(map[string]bool)
		for _, field := range toStringSlice(entry["model_writable_fields"]) {
			writable[field] = true
		}
		presets := make(map[string]map[string]any)
		if raw, ok := entry["presets"].(map[string]any); ok {
			for name, value := range raw {
				if parameters, ok := value.(map[string]any); ok {
					presets[name] = parameters
				}
			}
		}
		// The catalog's policy is part of the digest-bound authority: a
		// declared "requires_approval" intent must never auto-dispatch.
		requiresApproval := false
		if policy, ok := entry["policy"].(map[string]any); ok {
			if flag, ok := policy["requires_approval"].(bool); ok {
				requiresApproval = flag
			}
		}
		catalog.Entries[entryType] = &IntentEntry{
			Type:             entryType,
			RiskClass:        risk,
			ParameterSchema:  compiled,
			Presets:          presets,
			ModelWritable:    writable,
			RateLimitPerHour: intValue(entry["rate_limit"], "per_hour"),
			RequiresApproval: requiresApproval,
		}
	}
	return catalog, nil
}

func toStringSlice(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func intValue(section any, key string) int {
	raw, ok := section.(map[string]any)
	if !ok {
		return 0
	}
	if value, ok := raw[key].(float64); ok {
		return int(value)
	}
	return 0
}

type denyNetworkLoader struct{}

func (denyNetworkLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema load denied: %s", url)
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
	if len(rawIntents) == 0 {
		if decisionType, _ := document["decision_type"].(string); decisionType != "need_more_evidence" {
			return nil, reject("schema_invalid", "decision_type", "an empty-intent decision must explicitly request more evidence")
		}
	}
	// P4: at most ONE actionable (non-watch, non-compensation) intent — the
	// v1 "at most one actionable intent" contract is enforced independently,
	// not only by the worker's builder.
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
		validated, err := validateIntent(intent, input, decisionID, document, seenIDs)
		if err != nil {
			return nil, err
		}
		result.Intents = append(result.Intents, *validated)
	}
	return result, nil
}

func validateIntent(document map[string]any, input Input, decisionID string, decisionDocument map[string]any, seenIDs map[string]struct{}) (*Intent, error) {
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
	isCompensation := documentString(document, "compensates") != ""
	// The allowlist bypass is kind-scoped: only a RECONSIDER episode may carry
	// compensating intents. A DIAGNOSE worker forging compensates is rejected.
	if isCompensation && input.Kind != "reconsider" {
		return nil, reject("intent_type_not_allowed", "intent.compensates",
			"compensating intents require a reconsider episode")
	}
	if !isCompensation {
		if _, allowed := input.AllowedIntentTypes[intentType]; !allowed {
			return nil, reject("intent_type_not_allowed", "intent.type", "intent type is not allowed for this episode")
		}
	}
	// P4/B10: the catalog is the authority. The type must be declared; the
	// proposed risk must EQUAL the declared risk — a risk-label attack (the
	// worker claiming R0 for an R2 action) is rejected, not merely clamped to
	// the ceiling.
	entry, declared := input.IntentCatalog.Entries[intentType]
	if !declared {
		return nil, reject("intent_type_not_in_catalog", "intent.type", "intent type is not declared in the intent catalog")
	}
	risk, _ := document["risk_class"].(string)
	if risk != entry.RiskClass {
		return nil, reject("risk_label_mismatch", "intent.risk_class",
			fmt.Sprintf("proposed risk %s does not equal the declared %s for %s", risk, entry.RiskClass, intentType))
	}
	if riskRank(risk) > riskRank(input.RiskCeiling) {
		return nil, reject("risk_ceiling_exceeded", "intent.risk_class", "intent risk exceeds the episode ceiling")
	}
	// P4: parameters must satisfy the catalog's per-intent schema.
	parameters, _ := document["parameters"].(map[string]any)
	if err := entry.ParameterSchema.Validate(parameters); err != nil {
		return nil, reject("parameter_schema_violation", "intent.parameters", err.Error())
	}
	// P4: the builder-bound identity parameters are checked against the
	// dispatched episode — a tampered entity_id would otherwise flow into the
	// command payload unverified (the preset check only covers preset keys).
	if entityID, present := parameters["entity_id"]; present {
		if input.EntityID == "" {
			return nil, reject("snapshot_mismatch", "intent.parameters.entity_id",
				"the validator has no entity identity to bind against")
		}
		if entityID != input.EntityID {
			return nil, reject("snapshot_mismatch", "intent.parameters.entity_id",
				"intent entity does not match the dispatched episode")
		}
	}
	if expiresAt, present := parameters["expires_at"]; present {
		if validUntil, ok := decisionDocument["valid_until"].(string); ok && expiresAt != validUntil {
			return nil, reject("snapshot_mismatch", "intent.parameters.expires_at",
				"intent parameter expires_at must equal the decision's valid_until")
		}
	}
	if target, present := parameters["target"]; present {
		if entityID, ok := parameters["entity_id"]; ok && target != entityID {
			return nil, reject("snapshot_mismatch", "intent.parameters.target",
				"intent target must equal the bound entity")
		}
	}
	// P4: preset-only fields must be byte-identical to the compiled preset —
	// an attempt to silently substitute a preset value is rejected, not
	// repaired.
	if err := verifyPresetEquality(document, entry); err != nil {
		return nil, err
	}
	// P4: the intent's evidence must be grounded in the decision's facts.
	if err := verifyEvidenceBinding(document, decisionDocument); err != nil {
		return nil, err
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
		ID:               intentID,
		Type:             intentType,
		RiskClass:        risk,
		ExpiresAt:        expiresAt,
		Digest:           digest,
		CanonicalJSON:    canonical,
		Document:         document,
		RateLimitPerHour: entry.RateLimitPerHour,
		RequiresApproval: entry.RequiresApproval,
	}, nil
}

// verifyPresetEquality rejects any parameter whose key is NOT model-writable
// and IS present in the catalog's default preset but whose value differs from
// the preset's — the worker may only override the fields the catalog marks
// writable. Unknown keys (not in the preset, not writable) are schema-level
// noise and fail the schema check already.
func verifyPresetEquality(document map[string]any, entry *IntentEntry) error {
	parameters, _ := document["parameters"].(map[string]any)
	defaultPreset := entry.Presets["default"]
	for key, presetValue := range defaultPreset {
		if entry.ModelWritable[key] {
			continue
		}
		intentValue, present := parameters[key]
		if !present {
			continue
		}
		presetCanonical, err := canonicaljson.Marshal(presetValue)
		if err != nil {
			return reject("preset_mismatch", "intent.parameters."+key, err.Error())
		}
		intentCanonical, err := canonicaljson.Marshal(intentValue)
		if err != nil {
			return reject("preset_mismatch", "intent.parameters."+key, err.Error())
		}
		if string(presetCanonical) != string(intentCanonical) {
			return reject("preset_mismatch", "intent.parameters."+key,
				fmt.Sprintf("field %s is preset-authored, not model-writable", key))
		}
	}
	return nil
}

// verifyEvidenceBinding grounds the intent's evidence_ids in the decision's
// facts_used refs (P4/T3).
func verifyEvidenceBinding(document, decisionDocument map[string]any) error {
	evidenceIDs, _ := document["evidence_ids"].([]any)
	if len(evidenceIDs) == 0 {
		return nil
	}
	grounded := make(map[string]bool)
	if facts, ok := decisionDocument["facts_used"].([]any); ok {
		for _, fact := range facts {
			if object, ok := fact.(map[string]any); ok {
				if ref, ok := object["evidence"].(string); ok {
					grounded[ref] = true
				}
			}
		}
	}
	for _, ref := range evidenceIDs {
		text, ok := ref.(string)
		if !ok {
			return reject("parameter_schema_violation", "intent.evidence_ids", "evidence id must be a string")
		}
		if !grounded[text] {
			return reject("ungrounded_evidence", "intent.evidence_ids",
				fmt.Sprintf("evidence id %s is not among the decision's facts_used", text))
		}
	}
	return nil
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

func isExpired(now time.Time, value string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && !now.Before(parsed)
}
