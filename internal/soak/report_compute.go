package soak

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
)

func computeTx(ctx context.Context, tx *sql.Tx, tenantID string) (Report, error) {
	if tx == nil {
		return Report{}, fmt.Errorf("soak transaction is required")
	}
	report := Report{SchemaVersion: 1, Diagnostics: make(map[string]uint64)}
	if err := scanSafetyEvents(ctx, tx, &report); err != nil {
		return Report{}, err
	}
	setEvidenceRatio(&report)
	if err := addDiagnostics(ctx, tx, tenantID, &report); err != nil {
		return Report{}, err
	}
	setVerdict(&report)
	return report, nil
}

func scanSafetyEvents(ctx context.Context, tx *sql.Tx, report *Report) error {
	rows, err := tx.QueryContext(ctx, `SELECT event_id, event_type, details_json, details_sha256 FROM device_safety_events ORDER BY event_id`)
	if err != nil {
		return fmt.Errorf("query safety events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := countSafetyRow(rows, report); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate safety events: %w", err)
	}
	return nil
}

func decodeSafetyEvent(eventID int64, details, digest []byte) (map[string]any, error) {
	if err := deviceauthority.VerifyStoredJSONDigest(details, digest); err != nil {
		return nil, fmt.Errorf("verify safety event %d: %w", eventID, err)
	}
	var values map[string]any
	if err := json.Unmarshal(details, &values); err != nil {
		return nil, fmt.Errorf("decode safety event %d: %w", eventID, err)
	}
	return values, nil
}

func countSafetyEvent(report *Report, eventType string, values map[string]any) {
	if counter := report.ZeroTolerance.counter(eventType); counter != nil {
		*counter++
		return
	}
	if eventType == "physical_transition" {
		report.EvidenceCompleteness.Transitions++
		if deviceauthority.PhysicalEvidenceComplete(values) {
			report.EvidenceCompleteness.Complete++
		}
	}
}

func (counts *ZeroTolerance) counter(eventType string) *uint64 {
	return map[string]*uint64{
		"unsafe_output":                   &counts.UnsafeOutputCount,
		"stale_energizing_effect":         &counts.StaleEnergizingEffectCount,
		"duplicate_net_energizing_effect": &counts.DuplicateNetEnergizingCount,
		"unexplained_actuator_transition": &counts.UnexplainedActuatorTransition,
		"false_verified_success":          &counts.FalseVerifiedSuccessCount,
		"safe_state_deadline_miss":        &counts.SafeStateDeadlineMissCount,
	}[eventType]
}

func setEvidenceRatio(report *Report) {
	if report.EvidenceCompleteness.Transitions == 0 {
		report.EvidenceCompleteness.Ratio = 1
		return
	}
	report.EvidenceCompleteness.Ratio = float64(report.EvidenceCompleteness.Complete) / float64(report.EvidenceCompleteness.Transitions)
}

type diagnosticQuery struct {
	name  string
	query string
	args  []any
}

func addDiagnostics(ctx context.Context, tx *sql.Tx, tenantID string, report *Report) error {
	for _, item := range diagnosticQueries(tenantID) {
		var count int64
		if err := tx.QueryRowContext(ctx, item.query, item.args...).Scan(&count); err != nil {
			return fmt.Errorf("count %s: %w", item.name, err)
		}
		if count < 0 {
			return fmt.Errorf("count %s returned a negative value", item.name)
		}
		report.Diagnostics[item.name] = uint64(count)
	}
	return nil
}

func diagnosticQueries(tenantID string) []diagnosticQuery {
	if tenantID == "" {
		return globalDiagnosticQueries()
	}
	return tenantDiagnosticQueries(tenantID)
}

func globalDiagnosticQueries() []diagnosticQuery {
	return []diagnosticQuery{
		{name: "commands", query: "SELECT COUNT(*) FROM commands"},
		{name: "unknown_outcomes", query: "SELECT COUNT(*) FROM commands WHERE status IN ('outcome_unknown', 'reconciling')"},
		{name: "awaiting_verification", query: "SELECT COUNT(*) FROM verifications WHERE status = 'awaiting'"},
		{name: "unresolved_action_outcomes", query: `SELECT COUNT(*)
				FROM commands c LEFT JOIN verifications v ON v.command_id = c.command_id
				WHERE c.status IN ('outcome_unknown', 'reconciling') OR v.status = 'awaiting'`},
		{name: "reconciliation_barriers", query: "SELECT COUNT(*) FROM device_reconciliation WHERE status = 'required'"},
		{name: "authority_events", query: "SELECT COUNT(*) FROM device_authority_events"},
	}
}

func tenantDiagnosticQueries(tenantID string) []diagnosticQuery {
	return []diagnosticQuery{
		{name: "commands", query: "SELECT COUNT(*) FROM commands WHERE tenant_id = ?", args: []any{tenantID}},
		{name: "unknown_outcomes", query: "SELECT COUNT(*) FROM commands WHERE tenant_id = ? AND status IN ('outcome_unknown', 'reconciling')", args: []any{tenantID}},
		{name: "awaiting_verification", query: `SELECT COUNT(*) FROM verifications v JOIN commands c ON c.command_id = v.command_id
				WHERE c.tenant_id = ? AND v.status = 'awaiting'`, args: []any{tenantID}},
		{name: "unresolved_action_outcomes", query: `SELECT COUNT(*) FROM commands c LEFT JOIN verifications v ON v.command_id = c.command_id
				WHERE c.tenant_id = ? AND (c.status IN ('outcome_unknown', 'reconciling') OR v.status = 'awaiting')`, args: []any{tenantID}},
		{name: "reconciliation_barriers", query: "SELECT COUNT(*) FROM device_reconciliation WHERE status = 'required'"},
		{name: "authority_events", query: "SELECT COUNT(*) FROM device_authority_events"},
	}
}

func setVerdict(report *Report) {
	report.FailureReasons = failureReasons(*report)
	if len(report.FailureReasons) == 0 {
		report.Verdict = "pass"
		return
	}
	report.Verdict = "fail"
}

func failureReasons(report Report) []string {
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

func (report Report) failureCounts() map[string]uint64 {
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

func countSafetyRow(rows *sql.Rows, report *Report) error {
	var eventID int64
	var eventType string
	var details, digest []byte
	if err := rows.Scan(&eventID, &eventType, &details, &digest); err != nil {
		return fmt.Errorf("scan safety event %d: %w", eventID, err)
	}
	values, err := decodeSafetyEvent(eventID, details, digest)
	if err != nil {
		return err
	}
	countSafetyEvent(report, eventType, values)
	return nil
}
