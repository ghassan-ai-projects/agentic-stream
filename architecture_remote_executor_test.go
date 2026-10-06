package agenticstream

import (
	"go/ast"
	"path"
	"slices"
	"testing"
)

// TestRemoteExecutorFacadeOnlyDelegates confines worker protocol handling to
// the private layers.
func TestRemoteExecutorFacadeOnlyDelegates(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/executor/remote" {
			continue
		}
		for _, declaration := range parseGoFile(t, file).Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if !slices.Contains([]string{"NewExecutor", "NewExecutorWithEvidence", "Execute"}, function.Name.Name) || !facadeDelegationToApp(function) {
				t.Errorf("%s: %s must only delegate through the remote executor facade", file.rel, function.Name)
			}
		}
	}
}

// facadeDelegationToApp reports whether a facade function's body is one
// statement that reaches the private application (directly or via the field).
func facadeDelegationToApp(function *ast.FuncDecl) bool {
	return len(function.Body.List) == 1 && containsAppSelector(function.Body.List[0])
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
