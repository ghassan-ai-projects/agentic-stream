// Package soak derives the safety verdict for an exported run from durable
// evidence. Process-local telemetry is deliberately excluded from the
// verdict because it is reset on restart and cannot prove a physical event.
package soak

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

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

func computeTx(ctx context.Context, tx *sql.Tx, tenantID string) (Report, error) {
	if tx == nil {
		return Report{}, fmt.Errorf("soak transaction is required")
	}
	report := Report{SchemaVersion: 1, Diagnostics: make(map[string]uint64)}
	rows, err := tx.QueryContext(ctx, `SELECT event_id, event_type, details_json, details_sha256 FROM device_safety_events ORDER BY event_id`)
	if err != nil {
		return Report{}, fmt.Errorf("query safety events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var eventID int64
		var eventType string
		var details, digest []byte
		if err := rows.Scan(&eventID, &eventType, &details, &digest); err != nil {
			return Report{}, fmt.Errorf("scan safety event %d: %w", eventID, err)
		}
		if err := storage.VerifyStoredJSONDigest(details, digest); err != nil {
			return Report{}, fmt.Errorf("verify safety event %d: %w", eventID, err)
		}
		var values map[string]any
		if err := json.Unmarshal(details, &values); err != nil {
			return Report{}, fmt.Errorf("decode safety event %d: %w", eventID, err)
		}
		switch eventType {
		case "unsafe_output":
			report.ZeroTolerance.UnsafeOutputCount++
		case "stale_energizing_effect":
			report.ZeroTolerance.StaleEnergizingEffectCount++
		case "duplicate_net_energizing_effect":
			report.ZeroTolerance.DuplicateNetEnergizingCount++
		case "unexplained_actuator_transition":
			report.ZeroTolerance.UnexplainedActuatorTransition++
		case "false_verified_success":
			report.ZeroTolerance.FalseVerifiedSuccessCount++
		case "safe_state_deadline_miss":
			report.ZeroTolerance.SafeStateDeadlineMissCount++
		case "physical_transition":
			report.EvidenceCompleteness.Transitions++
			if storage.PhysicalEvidenceComplete(values) {
				report.EvidenceCompleteness.Complete++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return Report{}, fmt.Errorf("iterate safety events: %w", err)
	}
	if report.EvidenceCompleteness.Transitions == 0 {
		report.EvidenceCompleteness.Ratio = 1
	} else {
		report.EvidenceCompleteness.Ratio = float64(report.EvidenceCompleteness.Complete) / float64(report.EvidenceCompleteness.Transitions)
	}
	queries := map[string]struct {
		query string
		args  []any
	}{
		"commands":              {query: "SELECT COUNT(*) FROM commands"},
		"unknown_outcomes":      {query: "SELECT COUNT(*) FROM commands WHERE status IN ('outcome_unknown', 'reconciling')"},
		"awaiting_verification": {query: "SELECT COUNT(*) FROM verifications WHERE status = 'awaiting'"},
		"unresolved_action_outcomes": {query: `SELECT COUNT(*)
				FROM commands c LEFT JOIN verifications v ON v.command_id = c.command_id
				WHERE c.status IN ('outcome_unknown', 'reconciling') OR v.status = 'awaiting'`},
		"reconciliation_barriers": {query: "SELECT COUNT(*) FROM device_reconciliation WHERE status = 'required'"},
		"authority_events":        {query: "SELECT COUNT(*) FROM device_authority_events"},
	}
	if tenantID != "" {
		queries["commands"] = struct {
			query string
			args  []any
		}{"SELECT COUNT(*) FROM commands WHERE tenant_id = ?", []any{tenantID}}
		queries["unknown_outcomes"] = struct {
			query string
			args  []any
		}{"SELECT COUNT(*) FROM commands WHERE tenant_id = ? AND status IN ('outcome_unknown', 'reconciling')", []any{tenantID}}
		queries["awaiting_verification"] = struct {
			query string
			args  []any
		}{`SELECT COUNT(*) FROM verifications v JOIN commands c ON c.command_id = v.command_id
				WHERE c.tenant_id = ? AND v.status = 'awaiting'`, []any{tenantID}}
		queries["unresolved_action_outcomes"] = struct {
			query string
			args  []any
		}{`SELECT COUNT(*) FROM commands c LEFT JOIN verifications v ON v.command_id = c.command_id
				WHERE c.tenant_id = ? AND (c.status IN ('outcome_unknown', 'reconciling') OR v.status = 'awaiting')`, []any{tenantID}}
	}
	for name, item := range queries {
		var count int64
		if err := tx.QueryRowContext(ctx, item.query, item.args...).Scan(&count); err != nil {
			return Report{}, fmt.Errorf("count %s: %w", name, err)
		}
		if count < 0 {
			return Report{}, fmt.Errorf("count %s returned a negative value", name)
		}
		report.Diagnostics[name] = uint64(count)
	}
	report.FailureReasons = failureReasons(report)
	if len(report.FailureReasons) == 0 {
		report.Verdict = "pass"
	} else {
		report.Verdict = "fail"
	}
	return report, nil
}

func failureReasons(report Report) []string {
	reasons := make([]string, 0, 9)
	values := map[string]uint64{
		"unsafe_output_count":                   report.ZeroTolerance.UnsafeOutputCount,
		"stale_energizing_effect_count":         report.ZeroTolerance.StaleEnergizingEffectCount,
		"duplicate_net_energizing_effect_count": report.ZeroTolerance.DuplicateNetEnergizingCount,
		"unexplained_actuator_transition_count": report.ZeroTolerance.UnexplainedActuatorTransition,
		"false_verified_success_count":          report.ZeroTolerance.FalseVerifiedSuccessCount,
		"safe_state_deadline_miss_count":        report.ZeroTolerance.SafeStateDeadlineMissCount,
	}
	for name, value := range values {
		if value > 0 {
			reasons = append(reasons, fmt.Sprintf("%s=%d", name, value))
		}
	}
	if report.EvidenceCompleteness.Ratio < 1 {
		reasons = append(reasons, "evidence_completeness<1")
	}
	if count := report.Diagnostics["unresolved_action_outcomes"]; count > 0 {
		reasons = append(reasons, fmt.Sprintf("unresolved_action_outcomes=%d", count))
	}
	if count := report.Diagnostics["reconciliation_barriers"]; count > 0 {
		reasons = append(reasons, fmt.Sprintf("reconciliation_barriers=%d", count))
	}
	sort.Strings(reasons)
	return reasons
}
