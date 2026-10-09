package architecture

import (
	"go/ast"
	"slices"
	"strings"
	"testing"
)

// delegation decides whether a facade function only delegates.
type delegation func(*ast.FuncDecl) bool

// facadeSpec is the reviewed surface of one module's facade package.
// constructors are plain functions that may build the facade, exempt lists
// functions of any kind that are not operations, operations names every
// operation with the check it must pass, and other judges the functions that
// operations does not name; without it they are rejected.
type facadeSpec struct {
	module       string
	constructors []string
	exempt       []string
	operations   map[string]delegation
	other        delegation
}

type operationGroup struct {
	check delegation
	names []string
}

func where(check delegation, names ...string) operationGroup {
	return operationGroup{check: check, names: names}
}

func to(target string, names ...string) operationGroup {
	return where(delegatesTo(target), names...)
}

func operations(groups ...operationGroup) map[string]delegation {
	named := map[string]delegation{}
	for _, group := range groups {
		for _, name := range group.names {
			named[name] = group.check
		}
	}
	return named
}

func delegatesTo(target string) delegation {
	return func(function *ast.FuncDecl) bool { return facadeDelegation(function, target) }
}

func onMethods(check delegation) delegation {
	return func(function *ast.FuncDecl) bool { return function.Recv != nil && check(function) }
}

func onFunctions(check delegation) delegation {
	return func(function *ast.FuncDecl) bool { return function.Recv == nil && check(function) }
}

func singleStatement(function *ast.FuncDecl) bool {
	return function.Body != nil && len(function.Body.List) == 1
}

// reachesApp accepts one statement that uses the private application, directly
// or through the facade's field.
func reachesApp(function *ast.FuncDecl) bool {
	return singleStatement(function) && containsAppSelector(function.Body.List[0])
}

func containsAppSelector(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if selector, ok := n.(*ast.SelectorExpr); ok {
			if ident, ok := selector.X.(*ast.Ident); ok && ident.Name == "app" {
				found = true
			}
			if inner, ok := selector.X.(*ast.SelectorExpr); ok && inner.Sel.Name == "app" {
				found = true
			}
		}
		return !found
	})
	return found
}

// facadeDelegation reports whether the body is one return of a call through
// the named layer (a local name, or a field of that name).
func facadeDelegation(function *ast.FuncDecl, target string) bool {
	if function.Body == nil || len(function.Body.List) != 1 {
		return false
	}
	returned, ok := function.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	call, ok := returned.Results[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	switch receiver := selector.X.(type) {
	case *ast.Ident:
		return receiver.Name == target
	case *ast.SelectorExpr:
		return receiver.Sel.Name == target
	default:
		return false
	}
}

// domainEventOperation accepts the notify domain helpers: SourceForTenant and
// every function whose name ends in Event.
func domainEventOperation(function *ast.FuncDecl) bool {
	name := function.Name.Name
	return (name == "SourceForTenant" || strings.HasSuffix(name, "Event")) && facadeDelegation(function, "domain")
}

var facadeSpecs = []facadeSpec{
	{
		module:       "internal/actions",
		constructors: []string{"New", "NewReconciler", "leaseObserver"},
		operations: operations(
			to("app", "DispatchOnce", "Resolve", "Awaiting", "IntentCommands"),
			where(onFunctions(singleStatement), "CountUnresolvedOutcomes"),
		),
	},
	{
		module: "internal/approvalledger",
		operations: operations(to("app", "Request", "ExpireIntent", "Expire", "Resolve", "BindAssertion", "Withdraw", "WithdrawSuperseded",
			"Approvals", "PendingOfIntent", "LatestApprovedOfIntent", "PendingBinding")),
	},
	{
		module: "internal/canonicaljson",
		other:  delegatesTo("domain"),
	},
	{
		module:       "internal/cognition",
		constructors: []string{"New", "applicationConfig"},
		operations: operations(to("app", "Process", "RecordCostRejectionReason", "RecordSchedulerExpiryReason", "TriggerEvaluation",
			"TriggerEvaluations")),
	},
	{
		module: "internal/control",
		exempt: []string{"session", "utcNow", "NewDispatchAuthorization"},
		operations: operations(to("app", "Claim", "ClaimAndRecover", "Renew", "Release", "Assert", "Kill", "Drain", "State",
			"AssertDecision", "AssertDecisionTx", "AssertOrdinaryTx", "AssertAdmission", "Reserve", "Settle", "ApplyCostCeilings")),
	},
	{
		module:     "internal/decisions",
		operations: operations(to("domain", "Validate", "CompileIntentCatalog")),
	},
	{
		module:       "internal/engine",
		constructors: []string{"New", "applicationConfig", "ReplayOwnership"},
		operations:   operations(to("app", "RunGlobal", "ListSituations", "SituationVersion")),
	},
	{
		module: "internal/episodeledger",
		operations: operations(
			to("app", "Admit", "StartAttempt", "StartAttemptOwned", "TransitionAttempt", "RecordRejection", "RecoverUnfinishedAttempts",
				"Rebind", "BindRequest", "AbandonRebind", "Abandon", "Conclude", "RetainForRetry", "SupersedeEpoch", "SupersedeCoalesced",
				"UpsertSchedulerItem", "MarkSchedulerItemAdmitted", "CoalesceSchedulerItems", "CoalesceCostRejectedItem", "CoalesceSkippedItem",
				"DueSchedulerItems", "PollSchedulerQueue", "ExpireSchedulerItem", "Scheduling", "Episode",
				"ReadEpisodeFence", "ReadAttemptStatus", "ReadEpisodeLifecycle", "NextDispatchableEpisode", "ReadAdmission"),
			to("domain", "IsTerminalAttempt", "AttemptSQL"),
		),
	},
	{
		module:       "internal/episodes",
		constructors: []string{"New", "applicationConfig", "executionConfig"},
		operations: operations(
			where(onMethods(delegatesTo("app")), "Assemble", "Persist", "RunOnce"),
			where(onFunctions(delegatesTo("app")), "Decisions"),
			where(onFunctions(delegatesTo("domain")), "CompileIntentCatalog"),
		),
	},
	{
		module:       "internal/evidence",
		constructors: []string{"New", "applicationConfig", "ledgerConfig", "callConfig", "privateService"},
		operations: operations(
			to("app", "Issue", "Verify", "IssueTime", "RuntimeEpoch", "ReclaimExpired", "RecoverTx"),
			to("transport", "Call", "EventLogQuery"),
			to("wire", "NewRuntimeEpoch"),
		),
	},
	{
		module:       "internal/ingress",
		constructors: []string{"New", "applicationConfig"},
		operations:   operations(to("app", "ReplayJSONL", "ReplaySimulator", "ServeLive")),
	},
	{
		module: "internal/executor/native",
		operations: operations(where(singleStatement, "New", "NewMemoryArtifactStore", "NewSQLiteEvidenceTool", "RunBatch",
			"RunBatchJSON")),
	},
	{
		module:       "internal/notify",
		constructors: []string{"New"},
		operations:   operations(to("app", "ReadPage", "Prune", "Prunable", "Append", "AppendLifecycleEvent")),
		other:        domainEventOperation,
	},
	{
		module:     "internal/executor/remote",
		operations: operations(where(reachesApp, "NewExecutor", "NewExecutorWithEvidence", "Execute")),
	},
	{
		module:       "internal/runartifact",
		constructors: []string{"policyDocuments"},
		operations:   operations(to("app", "Export", "Verify")),
	},
	{
		module:       "internal/watch",
		constructors: []string{"New"},
		operations:   operations(to("app", "Dispatch", "DispatchAuthorized", "Expire", "FireEvent", "Watch", "InstalledWatchID")),
	},
}

func (s facadeSpec) accepts(function *ast.FuncDecl) bool {
	name := function.Name.Name
	if slices.Contains(s.exempt, name) || function.Recv == nil && slices.Contains(s.constructors, name) {
		return true
	}
	if check, named := s.operations[name]; named {
		return check(function)
	}
	return s.other != nil && s.other(function)
}

// TestFacadesOnlyDelegate keeps every module's public package a thin facade:
// each function is a reviewed operation that only delegates to the module's
// private application or domain layers (architecture-bar rule A12).
func TestFacadesOnlyDelegate(t *testing.T) {
	t.Parallel()

	repo := loadRepository(t)
	for _, spec := range facadeSpecs {
		t.Run(subtestName(spec.module), func(t *testing.T) {
			t.Parallel()

			for _, file := range repo.filesIn(t, spec.module) {
				for _, declaration := range file.syntax.Decls {
					if function, ok := declaration.(*ast.FuncDecl); ok && !spec.accepts(function) {
						t.Errorf("%s: %s must only delegate through the %s facade", file.rel, function.Name, spec.module)
					}
				}
			}
		})
	}
}

func subtestName(module string) string {
	return strings.ReplaceAll(strings.TrimPrefix(module, "internal/"), "/", "_")
}
