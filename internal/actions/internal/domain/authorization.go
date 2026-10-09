package domain

import (
	"errors"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

type IntentRow struct {
	ID, TenantID, DecisionID, SituationID string
	Version                               int
	Type, Risk, PolicyStatus              string
	ExpiresAt                             time.Time
	RequiresApproval                      bool
	JSON, SHA                             []byte
}

type ApprovalRow struct {
	ID        string
	ExpiresAt time.Time
	Present   bool
}

type DecisionRow struct {
	ValidationStatus, SituationID, EpisodeID string
	SituationVersion                         int
	JSON, SHA                                []byte
}

type EpisodeRow struct {
	TenantID, SituationID string
	SituationVersion      int
	ProducedDecision      bool
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
	raw, err := contractsv1.VerifyStoredDocument(contractsv1.SchemaCommand, canonicaljson.DomainCommand, r.Command.JSON, r.Command.SHA)
	if err != nil {
		return CommandDocument{}, errCommandIdentity
	}
	document := ParseCommandDocument(raw)
	if !document.MatchesLedger(r.Command) {
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
	switch contractsv1.RouteFor(contractsv1.RiskClass(r.Intent.Risk), r.Intent.RequiresApproval) {
	case contractsv1.RouteAutomatic:
		return nil
	case contractsv1.RouteApproval:
		return r.checkApprovalRecord(now)
	default:
		return errors.New("risk policy denies the intent")
	}
}

func (r AuthorizationRecords) checkApprovalRecord(now time.Time) error {
	if !r.Approval.Present {
		return errors.New("approved intent has no approved approval record")
	}
	if !r.Approval.ExpiresAt.After(now) {
		return errors.New("approval is expired")
	}
	return nil
}

func (r AuthorizationRecords) RequireCurrent() error {
	intent, episode := r.Intent, r.Episode
	if episode.TenantID == intent.TenantID && r.Situation.TenantID == intent.TenantID &&
		r.Decision.SituationID == intent.SituationID && r.Decision.SituationVersion == intent.Version &&
		episode.SituationID == intent.SituationID && episode.SituationVersion == intent.Version &&
		episode.ProducedDecision &&
		r.Situation.LastMaterialVersion <= intent.Version {
		return nil
	}
	return errors.New("command authorization is stale")
}

func (r AuthorizationRecords) CheckIntent(now time.Time) error {
	row := r.Intent
	if !row.ExpiresAt.After(now) {
		return errors.New("intent authorization is expired")
	}
	document, err := contractsv1.VerifyStoredDocument(contractsv1.SchemaIntent, canonicaljson.DomainIntent, row.JSON, row.SHA)
	if err != nil {
		return errors.New("intent authorization is invalid")
	}
	if !ParseIntentDocument(document).MatchesLedger(row) {
		return errors.New("intent authorization identity mismatch")
	}
	return nil
}

func (r AuthorizationRecords) CheckDecision() error {
	row := r.Decision
	document, err := contractsv1.VerifyStoredDocument(contractsv1.SchemaDecision, canonicaljson.DomainDecision, row.JSON, row.SHA)
	if err != nil {
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
