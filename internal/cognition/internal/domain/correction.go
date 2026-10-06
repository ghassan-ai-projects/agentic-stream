package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"time"
)

// ReconsiderationTrigger is the trigger name for corrected action evidence.
const ReconsiderationTrigger = "prior_action_invalidated"

// InvalidatedCommand is the latest successful action result invalidated by a correction.
type InvalidatedCommand struct {
	CommandID, DecisionID, OutcomeID, OutcomeStatus, ReconciliationStatus string
	OutcomeOrdinal                                                        int
	ProviderJSON, ObservedJSON, OutcomeSHA                                []byte
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

// DecodeCorrection validates and digests the persisted corrected snapshot.
func DecodeCorrection(snapshotJSON []byte) (map[string]any, string, error) {
	var correction map[string]any
	if err := json.Unmarshal(snapshotJSON, &correction); err != nil {
		return nil, "", fmt.Errorf("decode correction snapshot: %w", err)
	}
	if err := contractsv1.Validate(contractsv1.SchemaSnapshot, correction); err != nil {
		return nil, "", fmt.Errorf("validate correction snapshot: %w", err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainSnapshot, correction)
	if err != nil {
		return nil, "", fmt.Errorf("digest correction snapshot: %w", err)
	}
	return correction, digest, nil
}

// MatchCorrectionDigest decodes the expected digest and checks persisted bytes.
func MatchCorrectionDigest(digest string, persisted []byte) ([]byte, error) {
	decoded, err := canonicaljson.DecodeDigest(digest)
	if err != nil || !bytes.Equal(decoded, persisted) {
		return nil, fmt.Errorf("correction snapshot digest mismatch")
	}
	return decoded, nil
}

// NewReconsideration binds deterministic identities to one invalidated command.
func NewReconsideration(current situations.Version, command InvalidatedCommand) Reconsideration {
	key := ReconsiderationKey(current, command.CommandID)
	return Reconsideration{
		Current: current, Command: command,
		ID: ids.PrefixReconsideration + key, TriggerID: ReconsiderationTriggerID(key),
		SchedulerItemID: ReconsiderationSchedulerID(key),
	}
}

func ReconsiderationKey(current situations.Version, commandID string) string {
	material := fmt.Sprintf("reconsider|%s|%d|%s", current.SituationID, current.PreviousVersion, commandID)
	sum := sha256.Sum256([]byte(material))
	return hex.EncodeToString(sum[:])
}

func ReconsiderationTriggerID(key string) string { return "trg_reconsider_" + key }

func ReconsiderationSchedulerID(key string) string { return "sch_reconsider_" + key }

// EvidenceJSON returns canonical evidence for the reconsideration episode.
func (r Reconsideration) EvidenceJSON(correction map[string]any) ([]byte, error) {
	evidence := map[string]any{
		"reason": ReconsiderationTrigger, "correction": correction,
		"superseded_version": r.Current.PreviousVersion, "correction_version": r.Current.Version,
		"invalidated_command_id": r.Command.CommandID, "prior_decision_id": r.Command.DecisionID,
		"prior_outcome": r.Command.PriorOutcome(),
	}
	encoded, err := canonicaljson.Marshal(evidence)
	if err != nil {
		return nil, fmt.Errorf("canonicalize reconsideration evidence: %w", err)
	}
	return encoded, nil
}

// PriorOutcome projects the latest action result into reconsideration evidence.
func (c InvalidatedCommand) PriorOutcome() map[string]any {
	outcome := map[string]any{
		"status": c.OutcomeStatus, "reconciliation_status": c.ReconciliationStatus,
		"outcome_id": c.OutcomeID, "ordinal": c.OutcomeOrdinal,
		"outcome_sha256": "sha256:" + hex.EncodeToString(c.OutcomeSHA),
	}
	if len(c.ProviderJSON) > 0 {
		outcome["provider_result"] = json.RawMessage(c.ProviderJSON)
	}
	if len(c.ObservedJSON) > 0 {
		outcome["observed_effect"] = json.RawMessage(c.ObservedJSON)
	}
	return outcome
}

// reconsiderationEvaluation is the admitted deep-lane evaluation that
// explains why the Reconsideration exists.
func ReconsiderationEvaluation(r Reconsideration, deltaJSON []byte, policyDigest string, now time.Time) Evaluation {
	return Evaluation{
		TriggerID: r.TriggerID, TriggerName: ReconsiderationTrigger,
		SituationID: r.Current.SituationID, SituationVersion: r.Current.Version,
		Score: 100, Threshold: 0, Lane: "deep", Outcome: "admitted",
		Reasons:      []string{"accepted action invalidated by corrected Situation version"},
		PolicySHA256: policyDigest, DeltaJSON: deltaJSON, EvaluatedAt: now,
	}
}

func ReconsiderationItem(r Reconsideration, now time.Time) episodeledger.SchedulerItem {
	return episodeledger.SchedulerItem{
		SchedulerItemID: r.SchedulerItemID, Kind: "reconsider", TriggerID: r.TriggerID,
		SituationID: r.Current.SituationID, SituationVersion: r.Current.Version,
		Lane: "deep", Priority: 100, Status: "pending",
		ExpiresAt: ReconsiderationExpiry(now),
	}
}

func RequirePolicyDigest(digest string) error {
	if digest == "" {
		return fmt.Errorf("compiled spec has no digest")
	}
	return nil
}
