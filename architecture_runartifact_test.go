package agenticstream

import (
	"go/ast"
	"path"
	"slices"
	"testing"
)

// TestRunArtifactFacadeOnlyDelegates confines export, verification and file
// handling to private layers.
func TestRunArtifactFacadeOnlyDelegates(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/runartifact" {
			continue
		}
		for _, declaration := range parseGoFile(t, file).Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || (function.Recv == nil && function.Name.Name == "policyDocuments") {
				continue
			}
			if !slices.Contains([]string{"Export", "Verify"}, function.Name.Name) || !facadeDelegation(function, "app") {
				t.Errorf("%s: %s must only delegate through the run artifact facade", file.rel, function.Name)
			}
		}
	}
}

// TestRunArtifactStoreKeepsSnapshotOpaque protects the read-only snapshot's
// database handle.
func TestRunArtifactStoreKeepsSnapshotOpaque(t *testing.T) {
	t.Parallel()
	assertStoreEncapsulatesInfrastructure(t, "internal/runartifact/internal/store")
}

// TestRunArtifactApplicationUsesOpaquePorts keeps SQL and file calls behind
// named store and transport operations.
func TestRunArtifactApplicationUsesOpaquePorts(t *testing.T) {
	t.Parallel()
	assertApplicationUsesOpaquePorts(t, "internal/runartifact/internal/app")
}
