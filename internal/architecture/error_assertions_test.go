package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"slices"
	"strings"
	"testing"
)

// unnamedErrorAssertions is the burn-down baseline of test assertions that
// accept any error (`if err == nil { t.Fatal(...) }` with nothing naming the
// error), per test file. A file may only go down; rule T4 wants every error
// test to name the error it expects.
var unnamedErrorAssertions = map[string]int{
	"cmd/agentic-stream/commands_command_test.go":                   1,
	"cmd/agentic-stream/interlock_command_test.go":                  1,
	"cmd/agentic-stream/live_test.go":                               1,
	"cmd/agentic-stream/notifications_command_test.go":              1,
	"internal/actions/internal/domain/authorization_test.go":        1,
	"internal/actions/internal/domain/dispatch_test.go":             1,
	"internal/actions/internal/domain/reconciliation_test.go":       1,
	"internal/actions/internal/store/authorization_test.go":         3,
	"internal/actions/internal/store/transaction_test.go":           1,
	"internal/actions/reads_test.go":                                2,
	"internal/api/internal/domain/stream_test.go":                   2,
	"internal/approvalledger/internal/store/transition_test.go":     1,
	"internal/approvalledger/pending_lookup_test.go":                1,
	"internal/authority/internal/app/commands_test.go":              1,
	"internal/authority/internal/app/reconciliation_test.go":        4,
	"internal/authority/internal/app/safety_test.go":                3,
	"internal/authority/internal/app/service_test.go":               3,
	"internal/authority/internal/domain/evidence_test.go":           3,
	"internal/authority/internal/domain/reconciliation_test.go":     3,
	"internal/authority/internal/store/bindings_test.go":            1,
	"internal/canonicaljson/canonicaljson_test.go":                  3,
	"internal/canonicaljson/internal/domain/vectors_test.go":        1,
	"internal/cognition/internal/app/queue_timing_test.go":          1,
	"internal/cognition/internal/domain/trigger_test.go":            1,
	"internal/cognition/service_test.go":                            4,
	"internal/contractsv1/contractsv1_test.go":                      1,
	"internal/contractsv1/internal/domain/device_schema_test.go":    1,
	"internal/contractsv1/internal/domain/schemas_test.go":          1,
	"internal/contractsv1/internal/domain/stored_document_test.go":  1,
	"internal/control/internal/domain/rules_test.go":                4,
	"internal/control/internal/store/cost_test.go":                  1,
	"internal/decisions/facade_test.go":                             1,
	"internal/device/internal/app/session_reconciliation_test.go":   1,
	"internal/device/internal/transport/uds_frames_test.go":         1,
	"internal/engine/internal/app/contention_test.go":               1,
	"internal/engine/internal/app/rollback_test.go":                 1,
	"internal/engine/internal/domain/lateness_test.go":              1,
	"internal/engine/internal/domain/source_clocks_test.go":         1,
	"internal/engine/internal/domain/version_write_test.go":         1,
	"internal/engine/service_test.go":                               1,
	"internal/episodeledger/internal/store/attempt_test.go":         3,
	"internal/episodeledger/internal/store/fence_test.go":           1,
	"internal/episodes/internal/domain/evidence_horizon_test.go":    1,
	"internal/episodes/internal/domain/request_budget_test.go":      1,
	"internal/episodes/internal/store/lifecycle_test.go":            1,
	"internal/eventlog/quarantine_test.go":                          1,
	"internal/eventlog/schema_validation_test.go":                   1,
	"internal/evidence/facade_test.go":                              4,
	"internal/evidence/internal/app/capability_test.go":             1,
	"internal/evidence/internal/store/reservation_test.go":          4,
	"internal/evidence/internal/store/transaction_test.go":          1,
	"internal/evidence/internal/transport/event_log_query_test.go":  1,
	"internal/evidence/internal/wire/fixtures_test.go":              1,
	"internal/evidence/internal/wire/token_test.go":                 1,
	"internal/executor/remote/internal/domain/enum_mapping_test.go": 1,
	"internal/kernel/kernel_test.go":                                1,
	"internal/notify/append_test.go":                                1,
	"internal/notify/internal/app/append_test.go":                   1,
	"internal/notify/internal/app/prune_test.go":                    2,
	"internal/notify/internal/app/read_test.go":                     1,
	"internal/notify/internal/domain/lifecycle_contract_test.go":    5,
	"internal/notify/internal/domain/read_test.go":                  2,
	"internal/notify/internal/store/append_test.go":                 3,
	"internal/notify/lifecycle_events_test.go":                      1,
	"internal/notify/reading_test.go":                               1,
	"internal/notify/retention_test.go":                             2,
	"internal/operators/internal/domain/window_aggregate_test.go":   1,
	"internal/policy/definition_test.go":                            1,
	"internal/policy/internal/app/approval_expiry_test.go":          1,
	"internal/policy/internal/domain/commands_test.go":              1,
	"internal/policy/internal/domain/definition_test.go":            1,
	"internal/policy/internal/store/approval_context_test.go":       1,
	"internal/policy/internal/store/approval_principals_test.go":    2,
	"internal/policy/internal/store/commands_test.go":               3,
	"internal/policy/internal/store/intent_views_test.go":           2,
	"internal/policy/internal/store/pending_test.go":                1,
	"internal/policy/pending_test.go":                               1,
	"internal/policy/principals_test.go":                            1,
	"internal/runartifact/internal/domain/rules_test.go":            7,
	"internal/runartifact/internal/transport/directory_test.go":     1,
	"internal/runtime/internal/app/effect_routing_test.go":          1,
	"internal/runtime/internal/app/live_socket_test.go":             1,
	"internal/runtime/internal/app/pipeline_maintenance_test.go":    1,
	"internal/runtime/internal/app/scheduled_advance_test.go":       2,
	"internal/runtime/internal/app/service_test.go":                 7,
	"internal/runtime/internal/composition/ownership_test.go":       1,
	"internal/runtime/internal/store/admission_test.go":             1,
	"internal/runtime/internal/store/costs_test.go":                 2,
	"internal/runtime/internal/transport/sources_test.go":           6,
	"internal/runtime/pipeline_test.go":                             1,
	"internal/runtime/service_test.go":                              2,
	"internal/runtime/worker_runtime_test.go":                       2,
	"internal/storage/internal/store/fresh_database_test.go":        4,
	"internal/storage/storagetest/storagetest_test.go":              4,
	"internal/telemetry/internal/transport/endpoint_test.go":        1,
	"internal/telemetry/internal/transport/tracing_test.go":         1,
	"internal/testsupport/workerfake/dial_test.go":                  1,
	"internal/watch/internal/app/fire_test.go":                      1,
	"internal/watch/internal/domain/expression_test.go":             2,
	"internal/watch/internal/store/transaction_test.go":             1,
	"internal/worker/worker_test.go":                                2,
}

func TestErrorAssertionsNameTheErrorTheyExpect(t *testing.T) {
	t.Parallel()
	tests, err := loadRepository(t).tests()
	if err != nil {
		t.Fatalf("parse tests: %v", err)
	}
	found := make(map[string][]int)
	for _, file := range tests {
		if !strings.HasPrefix(file.rel, "internal/") && !strings.HasPrefix(file.rel, "cmd/") {
			continue
		}
		if lines := unnamedErrorChecks(file.syntax, loadRepository(t).fset); len(lines) > 0 {
			found[file.rel] = lines
		}
	}
	for _, rel := range slices.Sorted(maps.Keys(found)) {
		if len(found[rel]) > unnamedErrorAssertions[rel] {
			t.Errorf("%s: %d assertions accept any error (lines %v); assert which error with errors.Is, errors.As or its message (baseline %d)", rel, len(found[rel]), found[rel], unnamedErrorAssertions[rel])
		}
	}
	for rel, baseline := range unnamedErrorAssertions {
		if len(found[rel]) < baseline {
			t.Errorf("%s now has %d unnamed error assertions; lower its unnamedErrorAssertions baseline from %d", rel, len(found[rel]), baseline)
		}
	}
}

func unnamedErrorChecks(parsed *ast.File, fset *token.FileSet) []int {
	var lines []int
	ast.Inspect(parsed, func(node ast.Node) bool {
		ifStmt, ok := node.(*ast.IfStmt)
		if ok && acceptsAnyError(ifStmt.Cond) && failsTheTest(ifStmt.Body) {
			lines = append(lines, fset.Position(ifStmt.Pos()).Line)
		}
		return true
	})
	return lines
}

func acceptsAnyError(cond ast.Expr) bool {
	binary, ok := cond.(*ast.BinaryExpr)
	if !ok || binary.Op != token.EQL {
		return false
	}
	name, isIdent := binary.X.(*ast.Ident)
	nilValue, isNil := binary.Y.(*ast.Ident)
	return isIdent && isNil && nilValue.Name == "nil" && (name.Name == "err" || strings.HasSuffix(name.Name, "Err"))
}

func failsTheTest(body *ast.BlockStmt) bool {
	fails := false
	ast.Inspect(body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			if selector, isSelector := call.Fun.(*ast.SelectorExpr); isSelector {
				fails = strings.HasPrefix(selector.Sel.Name, "Fatal") || strings.HasPrefix(selector.Sel.Name, "Error")
			}
		}
		return !fails
	})
	return fails
}

func TestTheErrorAssertionDetectorSeesOnlyAssertionsThatAcceptAnyError(t *testing.T) {
	t.Parallel()
	source := `package p
func T(t *testing.T) {
	if err == nil { t.Fatal("x") }
	if err == nil || !errors.Is(err, io.EOF) { t.Fatal("x") }
	if writeErr == nil { t.Errorf("x") }
	if err != nil { t.Fatal(err) }
	if err == nil { return }
}`
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, "snippet_test.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(unnamedErrorChecks(parsed, fset)); got != "[3 5]" {
		t.Fatalf("detected lines %s, want [3 5]", got)
	}
}
