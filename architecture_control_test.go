package agenticstream

import (
	"go/ast"
	"path"
	"slices"
	"testing"
)

// controlOperations are the facade methods and functions; each delegates to the app layer.
var controlOperations = []string{
	"Claim", "ClaimAndRecover", "Renew", "Release", "Assert",
	"Kill", "Drain", "State", "AssertDecision", "AssertDecisionTx", "AssertOrdinaryTx", "AssertAdmission",
	"Reserve", "Settle", "SetCostLimit", "ApplyCostCeilings",
}

// TestControlFacadeOnlyDelegates confines control rules and SQL to private layers.
func TestControlFacadeOnlyDelegates(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/control" {
			continue
		}
		for _, declaration := range parseGoFile(t, file).Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || slices.Contains([]string{"session", "utcNow", "NewDispatchAuthorization"}, function.Name.Name) {
				continue
			}
			if !slices.Contains(controlOperations, function.Name.Name) || !facadeDelegation(function, "app") {
				t.Errorf("%s: %s must only delegate through the control facade", file.rel, function.Name)
			}
		}
	}
}

// TestControlStoreKeepsTransactionsOpaque protects the control store's database handle.
func TestControlStoreKeepsTransactionsOpaque(t *testing.T) {
	t.Parallel()
	assertStoreEncapsulatesInfrastructure(t, "internal/control/internal/store")
}

// TestControlApplicationUsesTransactionalPorts keeps SQL behind named store operations.
func TestControlApplicationUsesTransactionalPorts(t *testing.T) {
	t.Parallel()
	assertApplicationUsesOpaquePorts(t, "internal/control/internal/app")
}
