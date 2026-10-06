package agenticstream

import (
	"go/ast"
	"path"
	"slices"
	"testing"
)

// TestIngressFacadeOnlyDelegates confines replay, admission and socket code to private layers.
func TestIngressFacadeOnlyDelegates(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/ingress" {
			continue
		}
		for _, declaration := range parseGoFile(t, file).Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if function.Recv == nil && slices.Contains([]string{"New", "applicationConfig"}, function.Name.Name) {
				continue
			}
			if !slices.Contains([]string{"ReplayJSONL", "ReplaySimulator", "ServeLive"}, function.Name.Name) || !facadeDelegation(function, "app") {
				t.Errorf("%s: %s must only delegate through the ingress facade", file.rel, function.Name)
			}
		}
	}
}

// TestIngressStoreKeepsTransactionsOpaque protects the checkpoint store's database handle.
func TestIngressStoreKeepsTransactionsOpaque(t *testing.T) {
	t.Parallel()
	assertStoreEncapsulatesInfrastructure(t, "internal/ingress/internal/store")
}

// TestIngressApplicationUsesTransactionalPorts keeps SQL behind named store operations.
func TestIngressApplicationUsesTransactionalPorts(t *testing.T) {
	t.Parallel()
	assertApplicationUsesOpaquePorts(t, "internal/ingress/internal/app")
}
