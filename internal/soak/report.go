// Package soak derives the safety verdict for an exported run from durable
// evidence. Process-local telemetry is deliberately excluded from the
// verdict because it is reset on restart and cannot prove a physical event.
package soak

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
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

// Report is deterministic for one database snapshot. Diagnostics are
// report-only and cannot turn a failed safety counter into a pass.
type Report struct {
	SchemaVersion        int                  `json:"schema_version"`
	Verdict              string               `json:"verdict"`
	ZeroTolerance        ZeroTolerance        `json:"zero_tolerance"`
	EvidenceCompleteness EvidenceCompleteness `json:"evidence_completeness"`
	Diagnostics          map[string]uint64    `json:"diagnostics"`
	FailureReasons       []string             `json:"failure_reasons,omitempty"`
}

// Compute derives a report in one read transaction.
func Compute(ctx context.Context, db *storage.DB) (Report, error) {
	if db == nil {
		return Report{}, fmt.Errorf("soak database is required")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Report{}, fmt.Errorf("begin soak snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	report, err := ComputeTx(ctx, tx)
	if err != nil {
		return Report{}, err
	}
	if err := tx.Commit(); err != nil {
		return Report{}, fmt.Errorf("commit soak snapshot: %w", err)
	}
	return report, nil
}

// ComputeTx derives a report from the caller's consistent read transaction.
func ComputeTx(ctx context.Context, tx *sql.Tx) (Report, error) {
	return computeTx(ctx, tx, "")
}

// ComputeTenantTx derives a report for one tenant from the caller's consistent
// read transaction. Device authority and safety ledgers remain global because
// the current device tables do not carry a tenant column.
func ComputeTenantTx(ctx context.Context, tx *sql.Tx, tenantID string) (Report, error) {
	if tenantID == "" {
		return Report{}, fmt.Errorf("soak tenant is required")
	}
	return computeTx(ctx, tx, tenantID)
}
