package agenticstream

import (
	"go/ast"
	"path"
	"slices"
	"testing"
)

func assertStoreEncapsulatesInfrastructure(t *testing.T, pkg string) {
	t.Helper()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != pkg {
			continue
		}
		ast.Inspect(parseGoFile(t, file), func(node ast.Node) bool {
			declaration, ok := node.(*ast.TypeSpec)
			if !ok || !slices.Contains([]string{"Store", "Tx"}, declaration.Name.Name) {
				return true
			}
			record, ok := declaration.Type.(*ast.StructType)
			if declaration.Assign.IsValid() || !ok {
				t.Errorf("%s: %s must encapsulate infrastructure", file.rel, declaration.Name)
				return true
			}
			for _, field := range record.Fields.List {
				if len(field.Names) == 0 {
					t.Errorf("%s: %s embeds infrastructure", file.rel, declaration.Name)
				}
				for _, name := range field.Names {
					if name.IsExported() {
						t.Errorf("%s: infrastructure field %s must be private", file.rel, name)
					}
				}
			}
			return true
		})
	}
}

func assertApplicationUsesOpaquePorts(t *testing.T, pkg string) {
	t.Helper()
	raw := []string{"Exec", "ExecContext", "Query", "QueryContext", "QueryRow", "QueryRowContext", "Begin", "BeginTx", "Commit", "Rollback"}
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != pkg {
			continue
		}
		ast.Inspect(parseGoFile(t, file), func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if ok && slices.Contains(raw, selector.Sel.Name) {
				t.Errorf("%s: app calls raw infrastructure operation %s", file.rel, selector.Sel.Name)
			}
			return true
		})
	}
}
