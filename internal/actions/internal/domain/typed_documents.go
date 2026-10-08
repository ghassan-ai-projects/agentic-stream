package domain

import (
	"bytes"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// CommandDocument is the typed view of a command document, parsed once at the
// boundary. The raw Document stays the digest input; this view never feeds a
// digest.
type CommandDocument struct {
	CommandID, IntentID, TenantID, EffectorRoute, NormalizedTarget string
	IdempotencyKey, PolicyDigest                                   string
	NotBeforeMonoUS                                                int64
	Payload                                                        map[string]any
}

// ParseCommandDocument reads the command fields the dispatcher depends on.
func ParseCommandDocument(d map[string]any) CommandDocument {
	return CommandDocument{
		CommandID: contractsv1.DocumentString(d, "command_id"), IntentID: contractsv1.DocumentString(d, "intent_id"), TenantID: contractsv1.DocumentString(d, "tenant_id"),
		EffectorRoute: contractsv1.DocumentString(d, "effector_route"), NormalizedTarget: contractsv1.DocumentString(d, "normalized_target"),
		IdempotencyKey: contractsv1.DocumentString(d, "idempotency_key"), PolicyDigest: contractsv1.DocumentString(d, "policy_digest"),
		NotBeforeMonoUS: int64(contractsv1.DocumentInt(d, "not_before_mono_us")), Payload: payloadOf(d),
	}
}

func payloadOf(d map[string]any) map[string]any {
	payload, _ := d["payload"].(map[string]any)
	return payload
}

// MatchesLedger reports whether the document carries the ledger row's identity,
// including its idempotency key.
func (c CommandDocument) MatchesLedger(row CommandRow) bool {
	idempotency, err := canonicaljson.DecodeDigest(c.IdempotencyKey)
	return err == nil && row.ID == c.CommandID && row.IntentID == c.IntentID && row.TenantID == c.TenantID &&
		row.Route == c.EffectorRoute && row.Target == c.NormalizedTarget && bytes.Equal(row.Idempotency, idempotency)
}

// Command fills the effector command from the document, keeping the ledger
// command identity of base.
func (c CommandDocument) Command(base actionport.Command) actionport.Command {
	base.IntentID, base.TenantID = c.IntentID, c.TenantID
	base.EffectorRoute, base.NormalizedTarget = c.EffectorRoute, c.NormalizedTarget
	base.IdempotencyKey, base.PolicyDigest = c.IdempotencyKey, c.PolicyDigest
	base.NotBeforeMonoUS, base.Payload = c.NotBeforeMonoUS, c.Payload
	return base
}

// IntentDocument is the typed identity of an intent document.
type IntentDocument struct {
	IntentID, DecisionID, TenantID, SituationID, Type, RiskClass string
	SituationVersion                                             int
}

// ParseIntentDocument reads the intent identity fields.
func ParseIntentDocument(d map[string]any) IntentDocument {
	return IntentDocument{IntentID: contractsv1.DocumentString(d, "intent_id"), DecisionID: contractsv1.DocumentString(d, "decision_id"), TenantID: contractsv1.DocumentString(d, "tenant_id"),
		SituationID: contractsv1.DocumentString(d, "situation_id"), Type: contractsv1.DocumentString(d, "type"), RiskClass: contractsv1.DocumentString(d, "risk_class"), SituationVersion: contractsv1.DocumentInt(d, "situation_version")}
}

// MatchesLedger reports whether the document carries the ledger row's identity.
func (i IntentDocument) MatchesLedger(row IntentRow) bool {
	return i.IntentID == row.ID && i.DecisionID == row.DecisionID && i.TenantID == row.TenantID &&
		i.SituationID == row.SituationID && i.SituationVersion == row.Version && i.Type == row.Type && i.RiskClass == row.Risk
}

// DecisionDocument is the typed identity of a decision document.
type DecisionDocument struct {
	DecisionID, EpisodeID, SituationID string
	SituationVersion                   int
}

// ParseDecisionDocument reads the decision identity fields.
func ParseDecisionDocument(d map[string]any) DecisionDocument {
	return DecisionDocument{DecisionID: contractsv1.DocumentString(d, "decision_id"), EpisodeID: contractsv1.DocumentString(d, "episode_id"),
		SituationID: contractsv1.DocumentString(d, "situation_id"), SituationVersion: contractsv1.DocumentInt(d, "situation_version")}
}
