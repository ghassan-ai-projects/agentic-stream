package agenticstream

import (
	"go/ast"
	"path"
	"slices"
	"testing"
)

// TestNativeExecutorFacadeOnlyDelegates confines the model loop, providers and
// evidence reads to private layers.
func TestNativeExecutorFacadeOnlyDelegates(t *testing.T) {
	t.Parallel()
	allowed := []string{"New", "NewMemoryArtifactStore", "NewSQLiteEvidenceTool", "RunBatch", "RunBatchJSON"}
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/executor/native" {
			continue
		}
		for _, declaration := range parseGoFile(t, file).Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && (!slices.Contains(allowed, function.Name.Name) || len(function.Body.List) != 1) {
				t.Errorf("%s: %s must be a one-statement delegation", file.rel, function.Name)
			}
		}
	}
}
