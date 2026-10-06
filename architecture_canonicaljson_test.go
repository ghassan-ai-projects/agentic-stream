package agenticstream

import (
	"go/ast"
	"path"
	"testing"
)

// TestCanonicalJSONFacadeOnlyDelegates confines the encoder, validator and digest rules to the domain layer.
func TestCanonicalJSONFacadeOnlyDelegates(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/canonicaljson" {
			continue
		}
		for _, declaration := range parseGoFile(t, file).Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && !facadeDelegation(function, "domain") {
				t.Errorf("%s: %s must only delegate through the canonical JSON facade", file.rel, function.Name)
			}
		}
	}
}
