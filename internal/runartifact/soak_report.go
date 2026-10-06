package runartifact

// zeroTolerance is the Experiment 10 fail-closed counter set.
type zeroTolerance struct {
	UnsafeOutputCount             uint64 `json:"unsafe_output_count"`
	StaleEnergizingEffectCount    uint64 `json:"stale_energizing_effect_count"`
	DuplicateNetEnergizingCount   uint64 `json:"duplicate_net_energizing_effect_count"`
	UnexplainedActuatorTransition uint64 `json:"unexplained_actuator_transition_count"`
	FalseVerifiedSuccessCount     uint64 `json:"false_verified_success_count"`
	SafeStateDeadlineMissCount    uint64 `json:"safe_state_deadline_miss_count"`
}

// evidenceCompleteness describes whether every declared physical transition
// carried complete independent evidence.
type evidenceCompleteness struct {
	Transitions uint64  `json:"physical_transitions"`
	Complete    uint64  `json:"complete_transitions"`
	Ratio       float64 `json:"ratio"`
}

// soakReport is the safety verdict for one tenant's run, derived from durable
// evidence only: process-local telemetry is excluded because it resets on
// restart and cannot prove a physical event. It is deterministic for one
// database snapshot. Diagnostics are report-only and cannot turn a failed
// safety counter into a pass.
type soakReport struct {
	SchemaVersion        int                  `json:"schema_version"`
	Verdict              string               `json:"verdict"`
	ZeroTolerance        zeroTolerance        `json:"zero_tolerance"`
	EvidenceCompleteness evidenceCompleteness `json:"evidence_completeness"`
	Diagnostics          map[string]uint64    `json:"diagnostics"`
	FailureReasons       []string             `json:"failure_reasons,omitempty"`
}
