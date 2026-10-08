package agenticstream

import (
	"go/ast"
	"path"
	"slices"
	"testing"
)

// TestWatchFacadeOnlyDelegates confines watch rules and SQL to private layers.
func TestWatchFacadeOnlyDelegates(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/watch" {
			continue
		}
		for _, declaration := range parseGoFile(t, file).Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if function.Recv == nil && function.Name.Name == "New" {
				continue
			}
			if !slices.Contains([]string{"Dispatch", "DispatchAuthorized", "Expire", "FireEvent", "Watch"}, function.Name.Name) || !facadeDelegation(function, "app") {
				t.Errorf("%s: %s must only delegate through the watch facade", file.rel, function.Name)
			}
		}
	}
}

// TestWatchStoreKeepsTransactionsOpaque protects the mutating transaction.
func TestWatchStoreKeepsTransactionsOpaque(t *testing.T) {
	t.Parallel()
	assertStoreEncapsulatesInfrastructure(t, "internal/watch/internal/store")
}

// TestWatchApplicationUsesTransactionalPorts keeps SQL behind named store operations.
func TestWatchApplicationUsesTransactionalPorts(t *testing.T) {
	t.Parallel()
	assertApplicationUsesOpaquePorts(t, "internal/watch/internal/app")
}
