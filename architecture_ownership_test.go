package agenticstream

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// durableOwners pin every current production mutation, including shared handoff
// aggregates whose permitted phases are further restricted below.
var durableOwners = map[string]string{
	"approvals":                     "internal/approvalledger",
	"calibration_artifacts":         "internal/qualification",
	"commands":                      "internal/actions/internal/store",
	"connector_checkpoints":         "internal/ingress",
	"cost_limits":                   "internal/costcontrol",
	"cost_reservations":             "internal/costcontrol",
	"decisions":                     "internal/episodes/internal/store",
	"device_authority_events":       "internal/authority/internal/store",
	"device_command_bindings":       "internal/authority/internal/store",
	"device_reconciliation":         "internal/authority/internal/store",
	"device_safety_events":          "internal/authority/internal/store",
	"device_target_claims":          "internal/authority/internal/store",
	"episode_attempts":              "internal/episodeledger",
	"episode_rejections":            "internal/episodeledger",
	"episodes":                      "internal/episodeledger",
	"epoch_control":                 "internal/control",
	"event_gaps":                    "internal/eventlog/internal/store",
	"event_inbox":                   "internal/engine",
	"event_log":                     "internal/eventlog/internal/store",
	"event_quarantine":              "internal/eventlog/internal/store",
	"event_schemas":                 "internal/eventschema",
	"evidence_call_ledger":          "internal/evidence/internal/store",
	"intent_dispatch_counts":        "internal/policy/internal/store",
	"intents":                       "internal/policy/internal/store",
	"lineage_sets":                  "internal/engine",
	"notification_audits":           "internal/notify",
	"notification_cursors":          "internal/notify",
	"notification_event_tombstones": "internal/notify",
	"notification_poison_attempts":  "internal/notify",
	"notifications":                 "internal/notify",
	"operator_state":                "internal/engine",
	"outbox":                        "internal/actions/internal/store",
	"outcomes":                      "internal/actions/internal/store",
	"partition_checkpoints":         "internal/engine",
	"policy_evaluations":            "internal/policy/internal/store",
	"reconsiderations":              "internal/cognition/internal/store",
	"runtime_interlock":             "internal/interlock",
	"runtime_owner":                 "internal/control",
	"scheduler_items":               "internal/scheduleledger",
	"schema_migrations":             "internal/storage",
	"shadow_comparisons":            "internal/qualification",
	"shadow_decisions":              "internal/qualification",
	"situation_versions":            "internal/engine",
	"situations":                    "internal/engine",
	"spec_deployments":              "internal/spec",
	"timers":                        "internal/engine",
	"trigger_evaluations":           "internal/cognition/internal/store",
	"verifications":                 "internal/actions/internal/store",
	"watch_conditions":              "internal/watch/internal/store",
	"watch_fires":                   "internal/watch/internal/store",
}

func TestDurableMutationsHaveOneOwnerOrAnExplicitHandoffPhase(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	productionSQLMutations(t, func(file, position string, mutation sqlMutation) {
		pkg := filepath.ToSlash(filepath.Dir(file))
		seen[mutation.table] = true
		if durableOwners[mutation.table] == "" {
			t.Errorf("%s mutates unowned table %q", position, mutation.table)
			return
		}
		if !ownsMutation(pkg, mutation) {
			t.Errorf("%s: %s may not %s %s columns %v", position, pkg, mutation.operation, mutation.table, mutation.columns)
		}
		if file == "internal/control/dispatch_gate.go" {
			t.Errorf("%s: final authorization capability must be read-only", position)
		}
	})
	for table := range durableOwners {
		if !seen[table] {
			t.Errorf("stale durable owner declaration: %s", table)
		}
	}
}

func ownsMutation(pkg string, m sqlMutation) bool {
	switch m.table {
	case "commands", "outbox":
		// Policy may discard only its prepared, pending command before outbox publication.
		if m.table == "commands" && m.operation == "delete" {
			return pkg == "internal/policy/internal/store" && strings.EqualFold(strings.Join(strings.Fields(m.query), " "),
				"DELETE FROM commands WHERE intent_id = ? AND command_id = ? AND status = 'pending'")
		}
		if m.operation == "insert" {
			return pkg == "internal/policy/internal/store" && !m.rewritesExisting
		}
		if pkg != "internal/actions/internal/store" || m.operation != "update" {
			return false
		}
		if m.table == "commands" {
			return columnsWithin(m.columns, []string{"status", "updated_at"})
		}
		return columnsWithin(m.columns, []string{"status", "lease_owner", "lease_until", "attempt_count", "last_error_code", "delivered_at"})
	case "intents":
		if m.operation == "insert" {
			return pkg == "internal/episodes/internal/store" && !m.rewritesExisting
		}
		return pkg == "internal/policy/internal/store" && m.operation == "update" && columnsWithin(m.columns, []string{"policy_status", "updated_at"})
	case "situations":
		if pkg == "internal/cognition/internal/store" {
			return m.operation == "update" && columnsWithin(m.columns, []string{"last_reasoned_version"})
		}
	}
	return durableOwners[m.table] == pkg
}

func columnsWithin(actual, allowed []string) bool {
	if len(actual) == 0 {
		return false
	}
	for _, column := range actual {
		if !slices.Contains(allowed, column) {
			return false
		}
	}
	return true
}

func TestHandoffOwnershipRejectsAuthorityAndPayloadBypasses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ pkg, query string }{
		{"internal/cognition/internal/store", "UPDATE episodes SET lifecycle_status='superseded'"},
		{"internal/control", "UPDATE episodes SET lifecycle_status='superseded'"},
		{"internal/runtime", "UPDATE scheduler_items SET status='coalesced'"},
		{"internal/cognition/internal/store", "UPDATE approvals SET status='denied'"},
		{"internal/policy/internal/store", "UPDATE commands SET status='dispatching'"},
		{"internal/policy/internal/store", "DELETE FROM commands"},
		{"internal/actions", "INSERT INTO commands(command_id) VALUES ('bypass')"},
		{"internal/actions/internal/app", "UPDATE outbox SET status='delivered'"},
		{"internal/actions", "UPDATE commands SET command_json=?"},
		{"internal/actions/internal/store", "UPDATE commands SET command_json=?"},
		{"internal/actions/internal/store", "DELETE FROM outbox"},
		{"internal/policy/internal/store", "UPDATE intents SET intent_json=?"},
		{"internal/cognition/internal/store", "UPDATE situations SET current_version=2"},
		{"internal/episodes", "DELETE FROM intents"},
		{"internal/policy/internal/store", "REPLACE INTO commands(command_id) VALUES ('bypass')"},
		{"internal/policy/internal/store", "INSERT OR REPLACE INTO outbox(payload_json) VALUES ('bypass')"},
		{"internal/policy/internal/store", "INSERT INTO commands(command_id) VALUES ('bypass') ON CONFLICT DO UPDATE SET command_json=?"},
		{"internal/episodes", "INSERT INTO intents(intent_id) VALUES ('bypass') ON CONFLICT DO UPDATE SET policy_status='approved'"},
		{"internal/actions", "UPDATE commands SET status=?, (command_json,idempotency_key)=(?,?)"},
		{"internal/actions/internal/store", "UPDATE commands SET status=?, (command_json,idempotency_key)=(?,?)"},
	} {
		mutations := sqlMutations(tc.query)
		if len(mutations) != 1 {
			t.Fatalf("test query not classified: %s", tc.query)
		}
		if ownsMutation(tc.pkg, mutations[0]) {
			t.Errorf("ownership bypass accepted: %s: %s", tc.pkg, tc.query)
		}
	}
}
