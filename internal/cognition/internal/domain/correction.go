package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// ReconsiderationTrigger is the trigger name for corrected action evidence.
const ReconsiderationTrigger = "prior_action_invalidated"

// InvalidatedCommand is the latest successful action result invalidated by a correction.
type InvalidatedCommand struct {
	CommandID, DecisionID, OutcomeID, OutcomeStatus, ReconciliationStatus string
	CommandStatus, IntentID, IntentType, RiskClass                        string
	OutcomeOrdinal                                                        int
	ProviderJSON, ObservedJSON, OutcomeSHA, DecisionJSON, CommandJSON     []byte
}

// Reconsideration binds one corrected Situation to its invalidated command.
type Reconsideration struct {
	Current                        situations.Version
	Command                        InvalidatedCommand
	ID, TriggerID, SchedulerItemID string
}

// ShouldReconsider reports whether the corrected version uses this policy.
func ShouldReconsider(current situations.Version, latePolicy string) bool {
	return current.Completeness == "corrected" && current.PreviousVersion > 0 && latePolicy == "correct_and_reconsider"
}

func DecodeCorrection(snapshotJSON []byte) (map[string]any, error) {
	correction, err := contractsv1.DecodeDocument(snapshotJSON, contractsv1.SchemaSnapshot)
	if err != nil {
		return nil, fmt.Errorf("decode correction snapshot: %w", err)
	}
	return correction, nil
}

func MatchCorrectionDigest(correction map[string]any, persisted []byte) ([]byte, error) {
	if !contractsv1.VerifyDocumentDigest(canonicaljson.DomainSnapshot, correction, persisted) {
		return nil, fmt.Errorf("correction snapshot digest mismatch")
	}
	return persisted, nil
}

// NewReconsideration binds deterministic identities to one invalidated command.
func NewReconsideration(current situations.Version, command InvalidatedCommand) Reconsideration {
	key := ReconsiderationKey(current, command.CommandID)
	return Reconsideration{
		Current: current, Command: command,
		ID: sources.PrefixReconsideration + key, TriggerID: ReconsiderationTriggerID(key),
		SchedulerItemID: ReconsiderationSchedulerID(key),
	}
}

func ReconsiderationKey(current situations.Version, commandID string) string {
	material := fmt.Sprintf("reconsider|%s|%d|%s", current.SituationID, current.PreviousVersion, commandID)
	sum := sha256.Sum256([]byte(material))
	return hex.EncodeToString(sum[:])
}

func ReconsiderationTriggerID(key string) string {
	return sources.PrefixTrigger + "reconsider_" + key
}

func ReconsiderationSchedulerID(key string) string {
	return sources.PrefixScheduler + "reconsider_" + key
}

// EvidenceJSON returns canonical evidence for the reconsideration episode.
func (r Reconsideration) EvidenceJSON(correction map[string]any) ([]byte, error) {
	prior, err := r.Command.priorDocuments()
	if err != nil {
		return nil, err
	}
	encoded, err := canonicaljson.Marshal(r.evidence(correction, prior))
	if err != nil {
		return nil, fmt.Errorf("canonicalize reconsideration evidence: %w", err)
	}
	return encoded, nil
}

func (r Reconsideration) evidence(correction map[string]any, prior priorDocuments) map[string]any {
	return map[string]any{
		"reason": ReconsiderationTrigger, "correction": correction,
		"reconsideration_id": r.ID,
		"superseded_version": r.Current.PreviousVersion, "correction_version": r.Current.Version,
		"invalidated_command_id": r.Command.CommandID, "invalidated_outcome_id": r.Command.OutcomeID,
		"prior_decision_id": r.Command.DecisionID,
		"prior_decision":    prior.decision, "prior_command": prior.command, "prior_outcome": prior.outcome,
	}
}

func ReconsiderationEvaluation(r Reconsideration, deltaJSON []byte, policyDigest string, now time.Time) Evaluation {
	return reconsiderationOutcome(r, "admitted", "accepted action invalidated by corrected Situation version", deltaJSON, policyDigest, now)
}

func RejectedReconsiderationEvaluation(r Reconsideration, cause error, policyDigest string, now time.Time) Evaluation {
	return reconsiderationOutcome(r, "rejected", "prior documents unavailable: "+cause.Error(), []byte("{}"), policyDigest, now)
}

func reconsiderationOutcome(r Reconsideration, outcome, reason string, deltaJSON []byte, policyDigest string, now time.Time) Evaluation {
	return Evaluation{
		TriggerID: r.TriggerID, TriggerName: ReconsiderationTrigger,
		SituationID: r.Current.SituationID, SituationVersion: r.Current.Version,
		Score: 100, Threshold: 0, Lane: spec.LaneDeep, Outcome: outcome,
		Reasons:      []string{reason},
		PolicySHA256: policyDigest, DeltaJSON: deltaJSON, EvaluatedAt: now,
	}
}

func ReconsiderationItem(r Reconsideration, now time.Time) episodeledger.SchedulerItem {
	return episodeledger.SchedulerItem{
		SchedulerItemID: r.SchedulerItemID, Kind: episodeledger.KindReconsider, TriggerID: r.TriggerID,
		SituationID: r.Current.SituationID, SituationVersion: r.Current.Version,
		Lane: spec.LaneDeep, Priority: 100, Status: "pending",
		ExpiresAt: ReconsiderationExpiry(now),
	}
}

func RequirePolicyDigest(digest string) error {
	if digest == "" {
		return fmt.Errorf("compiled spec has no digest")
	}
	return nil
}
