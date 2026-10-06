package domain

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

// IntentRow is the ledger projection of the intent a command executes.
type IntentRow struct {
	ID, TenantID, DecisionID, SituationID string
	Version                               int
	Type, Risk, ExpiresAt, PolicyStatus   string
	JSON, SHA                             []byte
}

// ApprovalRow is the latest approved human approval of an intent. It is absent
// when the intent has no approved approval.
type ApprovalRow struct {
	ID, ExpiresAt string
	Present       bool
}

// DecisionRow is the ledger projection of the decision that produced the intent.
type DecisionRow struct {
	ValidationStatus, SituationID, EpisodeID string
	SituationVersion                         int
	JSON, SHA                                []byte
}

// EpisodeRow is the ledger projection of the episode that concluded the decision.
type EpisodeRow struct {
	TenantID, SituationID, Lifecycle string
	SituationVersion                 int
}

// SituationRow is the live Situation the intent is bound to.
type SituationRow struct {
	TenantID       string
	CurrentVersion int
}

// AuthorizationRecords is every ledger row a command's authority rests on.
type AuthorizationRecords struct {
	Command   CommandRow
	Intent    IntentRow
	Approval  ApprovalRow
	Decision  DecisionRow
	Episode   EpisodeRow
	Situation SituationRow
}

// VerifiedCommand decodes the command document and requires it to be
// schema-valid, digest-bound, and identical to its ledger columns.
func (r AuthorizationRecords) VerifiedCommand() (CommandDocument, error) {
	var raw Document
	row := r.Command
	if err := json.Unmarshal(row.JSON, &raw); err != nil || contractsv1.Validate(contractsv1.SchemaCommand, map[string]any(raw)) != nil ||
		!verifyDigest(canonicaljson.DomainCommand, raw, row.SHA) {
		return CommandDocument{}, errCommandIdentity
	}
	document := ParseCommandDocument(raw)
	if !document.MatchesLedger(row) {
		return CommandDocument{}, errCommandIdentity
	}
	return document, nil
}

var errCommandIdentity = errors.New("command ledger identity mismatch")

// RequireApprovedIntent requires the policy plane's approval of the command's
// intent and an accepted decision.
func (r AuthorizationRecords) RequireApprovedIntent() error {
	if r.Command.TenantID == r.Intent.TenantID && r.Command.IntentID == r.Intent.ID && r.Command.Route == r.Intent.Type &&
		r.Intent.PolicyStatus == "approved" && r.Decision.ValidationStatus == "accepted" {
		return nil
	}
	return errors.New("command is no longer approved for its intent")
}

// CheckApproval requires an unexpired human approval for an R2 intent.
func (r AuthorizationRecords) CheckApproval(now time.Time) error {
	if r.Intent.Risk != "R2" {
		return nil
	}
	if !r.Approval.Present {
		return errors.New("approved intent has no approved approval record")
	}
	approvalExpires, err := time.Parse(time.RFC3339Nano, r.Approval.ExpiresAt)
	if err != nil || !approvalExpires.After(now) {
		return errors.New("approval is expired")
	}
	return nil
}

// RequireCurrent requires the decision, episode and live Situation to still
// match the intent's tenant and Situation version.
func (r AuthorizationRecords) RequireCurrent() error {
	intent, episode := r.Intent, r.Episode
	if episode.TenantID == intent.TenantID && r.Situation.TenantID == intent.TenantID &&
		r.Decision.SituationID == intent.SituationID && r.Decision.SituationVersion == intent.Version &&
		episode.SituationID == intent.SituationID && episode.SituationVersion == intent.Version &&
		(episode.Lifecycle == "concluded" || episode.Lifecycle == "closed") &&
		r.Situation.CurrentVersion == intent.Version {
		return nil
	}
	return errors.New("command authorization is stale")
}

// CheckIntent requires an unexpired, digest-bound intent document that matches
// its ledger row.
func (r AuthorizationRecords) CheckIntent(now time.Time) error {
	row := r.Intent
	expiresAt, err := time.Parse(time.RFC3339Nano, row.ExpiresAt)
	if err != nil || !expiresAt.After(now) {
		return errors.New("intent authorization is expired")
	}
	var document Document
	if err := json.Unmarshal(row.JSON, &document); err != nil || contractsv1.Validate(contractsv1.SchemaIntent, map[string]any(document)) != nil || !verifyIntentDigest(document, row.SHA) {
		return errors.New("intent authorization is invalid")
	}
	if !ParseIntentDocument(document).MatchesLedger(row) {
		return errors.New("intent authorization identity mismatch")
	}
	return nil
}

// CheckDecision requires a digest-bound decision document that matches the
// intent's episode and Situation version.
func (r AuthorizationRecords) CheckDecision() error {
	row := r.Decision
	var document Document
	if err := json.Unmarshal(row.JSON, &document); err != nil || contractsv1.Validate(contractsv1.SchemaDecision, map[string]any(document)) != nil || !verifyDigest(canonicaljson.DomainDecision, document, row.SHA) {
		return errors.New("decision authorization is invalid")
	}
	if decision := ParseDecisionDocument(document); decision.DecisionID != r.Intent.DecisionID || decision.EpisodeID != row.EpisodeID ||
		decision.SituationID != r.Intent.SituationID || decision.SituationVersion != r.Intent.Version {
		return errors.New("decision authorization identity mismatch")
	}
	return nil
}

// CheckPolicyDigest requires the digest a command names to match the latest
// approving policy evaluation of its intent.
func CheckPolicyDigest(commanded, evaluated string) error {
	if commanded != evaluated {
		return errors.New("command policy digest is stale")
	}
	return nil
}
