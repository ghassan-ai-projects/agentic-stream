package agenticstream

import (
	"go/ast"
	"path"
	"slices"
	"testing"
)

// TestActionsFacadeOnlyDelegates confines dispatch and reconciliation to private layers.
func TestActionsFacadeOnlyDelegates(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/actions" {
			continue
		}
		for _, declaration := range parseGoFile(t, file).Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if function.Recv == nil && slices.Contains([]string{"New", "NewReconciler", "leaseObserver"}, function.Name.Name) {
				continue
			}
			if !actionsFacadeOperation(function) {
				t.Errorf("%s: %s must only delegate through the actions facade", file.rel, function.Name)
			}
		}
	}
}

// TestActionsStoreKeepsTransactionsOpaque protects the dispatch unit of work.
func TestActionsStoreKeepsTransactionsOpaque(t *testing.T) {
	t.Parallel()
	assertStoreEncapsulatesInfrastructure(t, "internal/actions/internal/store")
}

// TestActionsApplicationUsesTransactionalPorts keeps SQL behind named store operations.
func TestActionsApplicationUsesTransactionalPorts(t *testing.T) {
	t.Parallel()
	assertApplicationUsesOpaquePorts(t, "internal/actions/internal/app")
}

// actionsFacadeOperation accepts the two public operations as one-line delegates:
// DispatchOnce to the app service, CountUnresolvedOutcomes to the store join.
func actionsFacadeOperation(function *ast.FuncDecl) bool {
	switch function.Name.Name {
	case "DispatchOnce", "Resolve", "Awaiting", "IntentCommands":
		return facadeDelegation(function, "app")
	case "CountUnresolvedOutcomes":
		return function.Recv == nil && function.Body != nil && len(function.Body.List) == 1
	default:
		return false
	}
}
