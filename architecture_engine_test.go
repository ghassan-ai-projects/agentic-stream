package agenticstream

import (
	"go/ast"
	"path"
	"slices"
	"testing"
)

// TestEngineFacadeOnlyDelegates confines stream processing and SQL to private layers.
func TestEngineFacadeOnlyDelegates(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/engine" {
			continue
		}
		for _, declaration := range parseGoFile(t, file).Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if function.Recv == nil && slices.Contains([]string{"New", "applicationConfig", "ReplayOwnership"}, function.Name.Name) {
				continue
			}
			if function.Name.Name != "RunGlobal" || !facadeDelegation(function, "app") {
				t.Errorf("%s: %s must only delegate through the engine facade", file.rel, function.Name)
			}
		}
	}
}

// TestEngineStoreKeepsTransactionsOpaque protects the per-record transaction.
func TestEngineStoreKeepsTransactionsOpaque(t *testing.T) {
	t.Parallel()
	assertStoreEncapsulatesInfrastructure(t, "internal/engine/internal/store")
}

// TestEngineApplicationUsesTransactionalPorts keeps SQL behind named store operations.
func TestEngineApplicationUsesTransactionalPorts(t *testing.T) {
	t.Parallel()
	assertApplicationUsesOpaquePorts(t, "internal/engine/internal/app")
}
