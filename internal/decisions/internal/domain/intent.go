package domain

import (
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
)

func validateIntent(document map[string]any, input Input, decisionID string, decisionDocument map[string]any, seenIDs map[string]struct{}) (*Intent, error) {
	if err := checkIntentBinding(document, input, decisionID, seenIDs); err != nil {
		return nil, err
	}
	entry, err := checkIntentAuthority(document, input)
	if err != nil {
		return nil, err
	}
	if err := checkIntentParameters(document, input, decisionDocument, entry); err != nil {
		return nil, err
	}
	return buildIntent(document, input, entry)
}

// checkIntentBinding requires a unique intent bound to its Decision and to the
// episode's tenant, Situation, and version.
func checkIntentBinding(document map[string]any, input Input, decisionID string, seenIDs map[string]struct{}) error {
	intentID, _ := document["intent_id"].(string)
	if _, exists := seenIDs[intentID]; exists {
		return reject("schema_invalid", "intent_id", "Decision contains duplicate intent_id values")
	}
	seenIDs[intentID] = struct{}{}
	if got, _ := document["decision_id"].(string); got != decisionID {
		return reject("snapshot_mismatch", "intent.decision_id", "intent is not bound to its Decision")
	}
	return checkIntentSituation(document, input)
}

func checkIntentSituation(document map[string]any, input Input) error {
	if got, _ := document["tenant_id"].(string); got != input.TenantID {
		return reject("snapshot_mismatch", "intent.tenant_id", "intent tenant does not match the episode")
	}
	if got, _ := document["situation_id"].(string); got != input.SituationID {
		return reject("snapshot_mismatch", "intent.situation_id", "intent Situation does not match the episode")
	}
	if got, ok := integerField(document, "situation_version"); !ok || got != input.SituationVersion {
		return reject("snapshot_mismatch", "intent.situation_version", "intent Situation version does not match the episode")
	}
	return nil
}

// checkIntentAuthority checks the intent type against the episode allowlist
// and the catalog, and its risk against the catalog and the ceiling. It
// returns the catalog entry.
func checkIntentAuthority(document map[string]any, input Input) (*intentEntry, error) {
	intentType, _ := document["type"].(string)
	if err := checkIntentPermission(document, input, intentType); err != nil {
		return nil, err
	}
	// P4/B10: the catalog is the authority. The type must be declared; the
	// proposed risk must EQUAL the declared risk — a risk-label attack (the
	// worker claiming R0 for an R2 action) is rejected, not merely clamped to
	// the ceiling.
	entry, declared := input.IntentCatalog.entries[intentType]
	if !declared {
		return nil, reject("intent_type_not_in_catalog", "intent.type", "intent type is not declared in the intent catalog")
	}
	if err := checkIntentRisk(document, input, entry, intentType); err != nil {
		return nil, err
	}
	return entry, nil
}

func checkIntentPermission(document map[string]any, input Input, intentType string) error {
	isCompensation := contractsv1.DocumentString(document, "compensates") != ""
	// The allowlist bypass is kind-scoped: only a RECONSIDER episode may carry
	// compensating intents. A DIAGNOSE worker forging compensates is rejected.
	if isCompensation && !input.Reconsider {
		return reject("intent_type_not_allowed", "intent.compensates",
			"compensating intents require a reconsider episode")
	}
	if !isCompensation {
		if _, allowed := input.AllowedIntentTypes[intentType]; !allowed {
			return reject("intent_type_not_allowed", "intent.type", "intent type is not allowed for this episode")
		}
	}
	return nil
}

func checkIntentRisk(document map[string]any, input Input, entry *intentEntry, intentType string) error {
	risk, _ := document["risk_class"].(string)
	if risk != entry.RiskClass {
		return reject("risk_label_mismatch", "intent.risk_class",
			fmt.Sprintf("proposed risk %s does not equal the declared %s for %s", risk, entry.RiskClass, intentType))
	}
	if !contractsv1.RiskClass(risk).AtMost(contractsv1.RiskClass(input.RiskCeiling)) {
		return reject("risk_ceiling_exceeded", "intent.risk_class", "intent risk exceeds the episode ceiling")
	}
	return nil
}

// buildIntent checks the intent expiry and builds the validated Intent.
func buildIntent(document map[string]any, input Input, entry *intentEntry) (*Intent, error) {
	expiresAtString, _ := document["expires_at"].(string)
	expiresAt, err := kernel.ParseTime(expiresAtString)
	if err != nil {
		return nil, reject("schema_invalid", "intent.expires_at", err.Error())
	}
	if !expiresAt.After(input.Now) {
		return nil, reject("expired", "intent.expires_at", "intent has expired")
	}
	return materializeIntent(document, entry, expiresAt.UTC())
}

func materializeIntent(document map[string]any, entry *intentEntry, expiresAt time.Time) (*Intent, error) {
	canonical, err := canonicaljson.Marshal(document)
	if err != nil {
		return nil, reject("schema_invalid", "intent", err.Error())
	}
	digest, err := contractsv1.IntentDigest(document)
	if err != nil {
		return nil, reject("schema_invalid", "intent_digest", err.Error())
	}
	return validatedIntent(document, entry, expiresAt, canonical, digest), nil
}

func validatedIntent(document map[string]any, entry *intentEntry, expiresAt time.Time, canonical []byte, digest string) *Intent {
	intentID, _ := document["intent_id"].(string)
	intentType, _ := document["type"].(string)
	risk, _ := document["risk_class"].(string)
	return &Intent{
		ID:               intentID,
		Type:             intentType,
		RiskClass:        risk,
		ExpiresAt:        expiresAt,
		Digest:           digest,
		CanonicalJSON:    canonical,
		RateLimitPerHour: entry.RateLimitPerHour,
		RequiresApproval: entry.RequiresApproval,
	}
}
