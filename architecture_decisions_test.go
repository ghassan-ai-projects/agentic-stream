package agenticstream

import (
	"go/ast"
	"path"
	"slices"
	"testing"
)

// TestDecisionFacadeOnlyDelegates confines pure rules to the private domain.
func TestDecisionFacadeOnlyDelegates(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/decisions" {
			continue
		}
		for _, declaration := range parseGoFile(t, file).Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if !slices.Contains([]string{"Validate", "CompileIntentCatalog"}, function.Name.Name) || !facadeDelegation(function, "domain") {
				t.Errorf("%s: %s must only delegate to decisions domain", file.rel, function.Name)
			}
		}
	}
}

// TestDecisionCatalogKeepsAuthorityPrivate prevents public mutable compiled rules.
func TestDecisionCatalogKeepsAuthorityPrivate(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/decisions/internal/domain" {
			continue
		}
		ast.Inspect(parseGoFile(t, file), func(node ast.Node) bool {
			declaration, ok := node.(*ast.TypeSpec)
			if !ok || declaration.Name.Name != "IntentCatalog" {
				return true
			}
			record, ok := declaration.Type.(*ast.StructType)
			if declaration.Assign.IsValid() || !ok {
				t.Errorf("%s: catalog must encapsulate compiled authority", file.rel)
				return true
			}
			for _, field := range record.Fields.List {
				if len(field.Names) == 0 {
					t.Errorf("%s: catalog must not embed authority", file.rel)
				}
				for _, name := range field.Names {
					if name.IsExported() {
						t.Errorf("%s: catalog authority field %s must be private", file.rel, name)
					}
				}
			}
			return true
		})
	}
}
