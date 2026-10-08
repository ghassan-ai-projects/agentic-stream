package domain

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

type IntentRow struct {
	ID, TenantID, DecisionID, SituationID string
	Version                               int
	Type, Risk, ExpiresAt, PolicyStatus   string
	JSON, SHA                             []byte
}

type ApprovalRow struct {
	ID, ExpiresAt string
	Present       bool
}

type DecisionRow struct {
	ValidationStatus, SituationID, EpisodeID string
	SituationVersion                         int
	JSON, SHA                                []byte
}

type EpisodeRow struct {
	TenantID, SituationID, Lifecycle string
	SituationVersion                 int
}

type SituationRow struct {
	TenantID string

	LastMaterialVersion int
}

type AuthorizationRecords struct {
	Command   CommandRow
	Intent    IntentRow
	Approval  ApprovalRow
	Decision  DecisionRow
	Episode   EpisodeRow
	Situation SituationRow
}

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

func (r AuthorizationRecords) RequireApprovedIntent() error {
	if r.Command.TenantID == r.Intent.TenantID && r.Command.IntentID == r.Intent.ID && r.Command.Route == r.Intent.Type &&
		r.Intent.PolicyStatus == "approved" && r.Decision.ValidationStatus == "accepted" {
		return nil
	}
	return errors.New("command is no longer approved for its intent")
}

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

func (r AuthorizationRecords) RequireCurrent() error {
	intent, episode := r.Intent, r.Episode
	if episode.TenantID == intent.TenantID && r.Situation.TenantID == intent.TenantID &&
		r.Decision.SituationID == intent.SituationID && r.Decision.SituationVersion == intent.Version &&
		episode.SituationID == intent.SituationID && episode.SituationVersion == intent.Version &&
		(episode.Lifecycle == "concluded" || episode.Lifecycle == "closed") &&
		r.Situation.LastMaterialVersion <= intent.Version {
		return nil
	}
	return errors.New("command authorization is stale")
}

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

func CheckPolicyDigest(commanded, evaluated string) error {
	if commanded != evaluated {
		return errors.New("command policy digest is stale")
	}
	return nil
}
