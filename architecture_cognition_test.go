package agenticstream

import (
	"go/ast"
	"path"
	"slices"
	"testing"
)

// TestCognitionFacadeOnlyDelegates confines evaluation and scheduling to private layers.
func TestCognitionFacadeOnlyDelegates(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/cognition" {
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
			if !slices.Contains([]string{"Process", "RecordCostRejectionReason"}, function.Name.Name) || !facadeDelegation(function, "app") {
				t.Errorf("%s: %s must only delegate through the cognition facade", file.rel, function.Name)
			}
		}
	}
}

// TestCognitionStoreKeepsTransactionsOpaque protects the caller's original transaction.
func TestCognitionStoreKeepsTransactionsOpaque(t *testing.T) {
	t.Parallel()
	assertStoreEncapsulatesInfrastructure(t, "internal/cognition/internal/store")
}

// TestCognitionApplicationUsesTransactionalPorts keeps SQL behind named store operations.
func TestCognitionApplicationUsesTransactionalPorts(t *testing.T) {
	t.Parallel()
	assertApplicationUsesOpaquePorts(t, "internal/cognition/internal/app")
}
