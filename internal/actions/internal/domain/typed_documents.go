package domain

import (
	"bytes"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
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
func ParseCommandDocument(d Document) CommandDocument {
	return CommandDocument{
		CommandID: d.String("command_id"), IntentID: d.String("intent_id"), TenantID: d.String("tenant_id"),
		EffectorRoute: d.String("effector_route"), NormalizedTarget: d.String("normalized_target"),
		IdempotencyKey: d.String("idempotency_key"), PolicyDigest: d.String("policy_digest"),
		NotBeforeMonoUS: d.Int64("not_before_mono_us"), Payload: d.Object("payload"),
	}
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
func ParseIntentDocument(d Document) IntentDocument {
	return IntentDocument{IntentID: d.String("intent_id"), DecisionID: d.String("decision_id"), TenantID: d.String("tenant_id"),
		SituationID: d.String("situation_id"), Type: d.String("type"), RiskClass: d.String("risk_class"), SituationVersion: d.Int("situation_version")}
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
func ParseDecisionDocument(d Document) DecisionDocument {
	return DecisionDocument{DecisionID: d.String("decision_id"), EpisodeID: d.String("episode_id"),
		SituationID: d.String("situation_id"), SituationVersion: d.Int("situation_version")}
}
