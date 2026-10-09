package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

type ReconcilableCommand struct {
	ID, TenantID, IntentID, Target, Status string
	Trace                                  contractsv1.TraceContext
}

func (c ReconcilableCommand) RequireAwaitingReconciliation() error {
	if actionport.IsUnresolvedCommandStatus(c.Status) {
		return nil
	}
	return fmt.Errorf("command %s is not awaiting reconciliation", c.ID)
}

type ReconciledProvenance struct {
	Version int
	Digest  []byte
	Trace   contractsv1.TraceContext
}

func (p ReconciledProvenance) Validate() error {
	if p.Version < 1 || !canonicaljson.HasSumLength(p.Digest) {
		return errors.New("reconciled outcome provenance is incomplete")
	}
	return nil
}

type EvidenceType string

const (
	ProviderObservation EvidenceType = "provider_observation"
	DeviceStateFeedback EvidenceType = "device_state_feedback"
)

type Evidence struct {
	Source string
	Type   EvidenceType
	Raw    map[string]any
}

func ParseReconciliation(finalStatus string, raw map[string]any) (Evidence, error) {
	if finalStatus != actionport.CommandSucceeded && finalStatus != actionport.CommandFailed && finalStatus != actionport.CommandManualReview {
		return Evidence{}, fmt.Errorf("invalid reconciliation status %q", finalStatus)
	}
	if len(raw) == 0 {
		return Evidence{}, errors.New("reconciliation evidence is required")
	}
	evidence, err := evidenceHeader(raw)
	if err != nil {
		return Evidence{}, err
	}
	return evidence, evidence.validateTyped()
}

func evidenceHeader(raw map[string]any) (Evidence, error) {
	source, _ := raw["source"].(string)
	if source == "" {
		return Evidence{}, errors.New("reconciliation evidence source is required")
	}
	evidenceType, _ := raw["evidence_type"].(string)
	if EvidenceType(evidenceType) != ProviderObservation && EvidenceType(evidenceType) != DeviceStateFeedback {
		return Evidence{}, errors.New("reconciliation evidence_type is required")
	}
	return Evidence{Source: source, Type: EvidenceType(evidenceType), Raw: raw}, nil
}

func (e Evidence) validateTyped() error {
	if e.Type == DeviceStateFeedback {
		if err := requireDeviceFeedbackFields(e.Raw); err != nil {
			return err
		}
	}
	return requireEvidenceDigest(e.Raw)
}

func requireDeviceFeedbackFields(evidence map[string]any) error {
	for _, key := range []string{"device_id", "boot_id", "state", "feedback_digest"} {
		if _, ok := evidence[key]; !ok {
			return fmt.Errorf("device reconciliation evidence requires %s", key)
		}
	}
	return nil
}

func requireEvidenceDigest(evidence map[string]any) error {
	for _, key := range []string{"evidence_digest", "state_digest", "feedback_digest"} {
		digest, ok := evidence[key].(string)
		if !ok || digest == "" {
			continue
		}
		if _, err := canonicaljson.DecodeDigest(digest); err != nil {
			return fmt.Errorf("invalid reconciliation %s: %w", key, err)
		}
		return nil
	}
	return errors.New("reconciliation evidence must include a sha256 evidence, state, or feedback digest")
}

func ReconciledVerificationStatus(finalStatus string) string {
	switch finalStatus {
	case actionport.CommandSucceeded:
		return actionport.VerificationReconciled
	case actionport.CommandFailed:
		return actionport.VerificationRefuted
	default:
		return actionport.VerificationInconclusive
	}
}

func Verdict(finalStatus string) string {
	switch finalStatus {
	case actionport.CommandSucceeded:
		return "verified"
	case actionport.CommandFailed:
		return "refuted"
	default:
		return "inconclusive"
	}
}

func ReconciliationOutcomeDocument(commandID, outcomeID, finalStatus string, evidence map[string]any, at time.Time) Document {
	return OutcomeDocument(commandID, outcomeID, OutcomeReconciled, map[string]any{"final_status": finalStatus, "evidence": evidence}, "", at)
}

type AwaitingCommand struct {
	CommandID string `json:"command_id"`
	IntentID  string `json:"intent_id"`
	Route     string `json:"route"`
	Target    string `json:"target"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}
