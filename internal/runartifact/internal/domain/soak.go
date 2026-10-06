package domain

import (
	"fmt"
	"sort"
)

// ZeroTolerance is the Experiment 10 fail-closed counter set.
type ZeroTolerance struct {
	UnsafeOutputCount             uint64 `json:"unsafe_output_count"`
	StaleEnergizingEffectCount    uint64 `json:"stale_energizing_effect_count"`
	DuplicateNetEnergizingCount   uint64 `json:"duplicate_net_energizing_effect_count"`
	UnexplainedActuatorTransition uint64 `json:"unexplained_actuator_transition_count"`
	FalseVerifiedSuccessCount     uint64 `json:"false_verified_success_count"`
	SafeStateDeadlineMissCount    uint64 `json:"safe_state_deadline_miss_count"`
}

// EvidenceCompleteness describes whether every declared physical transition
// carried complete independent evidence.
type EvidenceCompleteness struct {
	Transitions uint64  `json:"physical_transitions"`
	Complete    uint64  `json:"complete_transitions"`
	Ratio       float64 `json:"ratio"`
}

// SoakReport is the safety verdict for one tenant's run, derived from durable
// evidence only: process-local telemetry resets on restart and cannot prove a
// physical event. Diagnostics are report-only and cannot turn a failed safety
// counter into a pass.
type SoakReport struct {
	SchemaVersion        int                  `json:"schema_version"`
	Verdict              string               `json:"verdict"`
	ZeroTolerance        ZeroTolerance        `json:"zero_tolerance"`
	EvidenceCompleteness EvidenceCompleteness `json:"evidence_completeness"`
	Diagnostics          map[string]uint64    `json:"diagnostics"`
	FailureReasons       []string             `json:"failure_reasons,omitempty"`
}

// SafetyEvidence is the device authority's durable safety tally and the action
// plane's outcome counts, as the store read them.
type SafetyEvidence struct {
	ZeroTolerance       ZeroTolerance
	PhysicalTransitions uint64
	CompleteTransitions uint64
	OpenReconciliations uint64
	AuthorityEvents     uint64
	ActionDiagnostics   map[string]uint64
}

// DeriveSoakReport turns the evidence into the report and its verdict.
func DeriveSoakReport(evidence SafetyEvidence) SoakReport {
	report := SoakReport{
		SchemaVersion: 1,
		ZeroTolerance: evidence.ZeroTolerance,
		Diagnostics:   map[string]uint64{"reconciliation_barriers": evidence.OpenReconciliations, "authority_events": evidence.AuthorityEvents},
		EvidenceCompleteness: EvidenceCompleteness{
			Transitions: evidence.PhysicalTransitions, Complete: evidence.CompleteTransitions,
			Ratio: completenessRatio(evidence.PhysicalTransitions, evidence.CompleteTransitions),
		},
	}
	for name, count := range evidence.ActionDiagnostics {
		report.Diagnostics[name] = count
	}
	report.FailureReasons = failureReasons(report)
	report.Verdict = verdictFor(report.FailureReasons)
	return report
}

func completenessRatio(transitions, complete uint64) float64 {
	if transitions == 0 {
		return 1
	}
	return float64(complete) / float64(transitions)
}

func verdictFor(reasons []string) string {
	if len(reasons) == 0 {
		return "pass"
	}
	return "fail"
}

func failureReasons(report SoakReport) []string {
	reasons := make([]string, 0, 9)
	for name, value := range report.failureCounts() {
		if value > 0 {
			reasons = append(reasons, fmt.Sprintf("%s=%d", name, value))
		}
	}
	if report.EvidenceCompleteness.Ratio < 1 {
		reasons = append(reasons, "evidence_completeness<1")
	}
	sort.Strings(reasons)
	return reasons
}

func (report SoakReport) failureCounts() map[string]uint64 {
	return map[string]uint64{
		"unsafe_output_count":                   report.ZeroTolerance.UnsafeOutputCount,
		"stale_energizing_effect_count":         report.ZeroTolerance.StaleEnergizingEffectCount,
		"duplicate_net_energizing_effect_count": report.ZeroTolerance.DuplicateNetEnergizingCount,
		"unexplained_actuator_transition_count": report.ZeroTolerance.UnexplainedActuatorTransition,
		"false_verified_success_count":          report.ZeroTolerance.FalseVerifiedSuccessCount,
		"safe_state_deadline_miss_count":        report.ZeroTolerance.SafeStateDeadlineMissCount,
		"unresolved_action_outcomes":            report.Diagnostics["unresolved_action_outcomes"],
		"reconciliation_barriers":               report.Diagnostics["reconciliation_barriers"],
	}
}
