package agenticstream

import (
	"go/ast"
	"path"
	"slices"
	"strings"
	"testing"
)

// TestEpisodeFacadeOnlyDelegates keeps public operations out of the lifecycle implementation.
func TestEpisodeFacadeOnlyDelegates(t *testing.T) {
	t.Parallel()
	operations := []string{"Assemble", "Persist", "RunOnce"}
	construction := []string{"New", "applicationConfig", "executionConfig"}
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/episodes" {
			continue
		}
		for _, declaration := range parseGoFile(t, file).Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if function.Recv != nil {
				if !slices.Contains(operations, function.Name.Name) || !facadeDelegation(function, "app") {
					t.Errorf("%s: %s must only delegate to the private application", file.rel, function.Name)
				}
			} else if function.Name.Name == "CompileIntentCatalog" {
				if !facadeDelegation(function, "domain") {
					t.Errorf("%s: catalog compilation must delegate to domain", file.rel)
				}
			} else if !slices.Contains(construction, function.Name.Name) {
				t.Errorf("%s: %s is implementation outside the facade's configuration surface", file.rel, function.Name)
			}
		}
	}
}

func facadeDelegation(function *ast.FuncDecl, target string) bool {
	if function.Body == nil || len(function.Body.List) != 1 {
		return false
	}
	returned, ok := function.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	call, ok := returned.Results[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	switch receiver := selector.X.(type) {
	case *ast.Ident:
		return receiver.Name == target
	case *ast.SelectorExpr:
		return receiver.Sel.Name == target
	default:
		return false
	}
}

// TestEpisodeStoreKeepsTransactionsOpaque prevents SQL aliases and public DB handles reaching app.
func TestEpisodeStoreKeepsTransactionsOpaque(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/episodes/internal/store" {
			continue
		}
		ast.Inspect(parseGoFile(t, file), func(node ast.Node) bool {
			declaration, ok := node.(*ast.TypeSpec)
			if !ok || !slices.Contains([]string{"Store", "Tx"}, declaration.Name.Name) {
				return true
			}
			if declaration.Assign.IsValid() {
				t.Errorf("%s: %s must be opaque, never an alias", file.rel, declaration.Name)
			}
			record, ok := declaration.Type.(*ast.StructType)
			if !ok {
				t.Errorf("%s: %s must encapsulate infrastructure in private fields", file.rel, declaration.Name)
				return true
			}
			for _, field := range record.Fields.List {
				if len(field.Names) == 0 {
					t.Errorf("%s: %s embeds infrastructure", file.rel, declaration.Name)
				}
				for _, name := range field.Names {
					if name.IsExported() {
						t.Errorf("%s: %s exposes infrastructure field %s", file.rel, declaration.Name, name)
					}
				}
			}
			return true
		})
	}
}

// TestEpisodeApplicationUsesTransactionalPorts prevents raw infrastructure calls even through aliases.
func TestEpisodeApplicationUsesTransactionalPorts(t *testing.T) {
	t.Parallel()
	rawSQL := []string{"Exec", "ExecContext", "Query", "QueryContext", "QueryRow", "QueryRowContext", "Begin", "BeginTx", "Commit", "Rollback"}
	module := readModulePath(t, path.Join(repoRoot(t), "go.mod"))
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/episodes/internal/app" {
			continue
		}
		parsed := parseGoFile(t, file)
		aliases := map[string]string{}
		for _, imported := range parsed.Imports {
			name := path.Base(strings.Trim(imported.Path.Value, `"`))
			if imported.Name != nil {
				name = imported.Name.Name
			}
			aliases[name] = strings.TrimPrefix(strings.Trim(imported.Path.Value, `"`), module+"/")
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if slices.Contains(rawSQL, selector.Sel.Name) {
				t.Errorf("%s: app calls raw SQL operation %s", file.rel, selector.Sel.Name)
			}
			receiver, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			owner := aliases[receiver.Name]
			if owner == "internal/episodeledger" && !slices.Contains([]string{"IsTerminalAttempt", "RejectionReason"}, selector.Sel.Name) {
				t.Errorf("%s: app calls transactional owner %s.%s directly", file.rel, owner, selector.Sel.Name)
			}
			return true
		})
	}
}
