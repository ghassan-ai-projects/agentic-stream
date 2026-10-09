package architecture

import (
	"go/ast"
	"os"
	"path"
	"path/filepath"
	"slices"
	"testing"
)

// moduleShapeExceptions are the stand-alone packages that do not carry a domain
// layer, each with the reason it must not: see
// docs/remaining-migration-2026-10-06/UNIFORM_ARCHITECTURE.md.
var moduleShapeExceptions = map[string]string{
	"internal/architecture":     "repository-wide gates: no production code beyond its package comment (README.md)",
	"internal/executor/fixture": "demo executor used by composition; held for the owner",
	"internal/kernel":           "shared pure vocabulary with no rules of its own to separate; one package by design (TestKernelHasNoSubpackages)",
}

// TestEveryModuleHasAFacadeAndADomain requires each module to be a facade over
// internal/domain, so the exposed surface is a deliberate choice and the rules
// have one home. Exceptions are listed above with their reason.
func TestEveryModuleHasAFacadeAndADomain(t *testing.T) {
	t.Parallel()

	root := loadRepository(t).root
	for _, module := range moduleDirectories(t, root) {
		requireDomain(t, root, module)
	}
}

func requireDomain(t *testing.T, root, module string) {
	t.Helper()

	rel, _ := filepath.Rel(root, module)
	rel = filepath.ToSlash(rel)
	if !hasProductionGoFiles(module) || moduleShapeExceptions[rel] != "" {
		return
	}
	if info, err := os.Stat(filepath.Join(module, "internal", "domain")); err != nil || !info.IsDir() {
		t.Errorf("%s has no internal/domain layer; give it one or list it in moduleShapeExceptions with a reason", rel)
	}
}

// opaqueStoreModules are the modules whose store layer keeps its database
// handle and transactions private (architecture-bar rule A12).
var opaqueStoreModules = []string{
	"internal/actions", "internal/approvalledger", "internal/cognition", "internal/control", "internal/engine",
	"internal/episodeledger", "internal/episodes", "internal/evidence", "internal/ingress", "internal/notify",
	"internal/runartifact", "internal/watch",
}

// TestStoresKeepTransactionsOpaque requires the Store and Tx types of a store
// layer to be structs with private fields, never an alias of a database handle,
// so no layer above can reach raw SQL through them.
func TestStoresKeepTransactionsOpaque(t *testing.T) {
	t.Parallel()

	repo := loadRepository(t)
	for _, module := range opaqueStoreModules {
		t.Run(subtestName(module), func(t *testing.T) {
			t.Parallel()

			for _, file := range repo.filesIn(t, module+"/internal/store") {
				ast.Inspect(file.syntax, func(node ast.Node) bool {
					reportLeakyStoreType(t, file, node)
					return true
				})
			}
		})
	}
}

func reportLeakyStoreType(t *testing.T, file goFile, node ast.Node) {
	t.Helper()

	declaration, ok := node.(*ast.TypeSpec)
	if !ok || !slices.Contains([]string{"Store", "Tx"}, declaration.Name.Name) {
		return
	}
	record, ok := declaration.Type.(*ast.StructType)
	if declaration.Assign.IsValid() || !ok {
		t.Errorf("%s: %s must encapsulate infrastructure", file.rel, declaration.Name)
		return
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
}

// rawInfrastructureCalls are database and transaction methods an application
// layer must not call; it goes through its store's named operations.
var rawInfrastructureCalls = []string{"Exec", "ExecContext", "Query", "QueryContext", "QueryRow", "QueryRowContext", "Begin", "BeginTx", "Commit", "Rollback"}

// applicationLayerSpec names the raw calls one module's application layer avoids.
type applicationLayerSpec struct {
	module string
	avoids []string
}

// applicationLayerSpecs lists every module's application layer. The evidence
// layer is allowed Query because its own service type defines a Query method.
var applicationLayerSpecs = []applicationLayerSpec{
	{"internal/actions", rawInfrastructureCalls},
	{"internal/approvalledger", rawInfrastructureCalls},
	{"internal/cognition", rawInfrastructureCalls},
	{"internal/control", rawInfrastructureCalls},
	{"internal/engine", rawInfrastructureCalls},
	{"internal/episodeledger", rawInfrastructureCalls},
	{"internal/episodes", rawInfrastructureCalls},
	{"internal/evidence", slices.DeleteFunc(slices.Clone(rawInfrastructureCalls), func(call string) bool { return call == "Query" })},
	{"internal/ingress", rawInfrastructureCalls},
	{"internal/notify", rawInfrastructureCalls},
	{"internal/runartifact", rawInfrastructureCalls},
	{"internal/watch", rawInfrastructureCalls},
}

// TestApplicationsUseOpaquePorts keeps SQL and transactions behind the named
// operations of the module's store, even when reached through an alias.
func TestApplicationsUseOpaquePorts(t *testing.T) {
	t.Parallel()

	repo := loadRepository(t)
	for _, spec := range applicationLayerSpecs {
		t.Run(subtestName(spec.module), func(t *testing.T) {
			t.Parallel()

			for _, file := range repo.filesIn(t, path.Join(spec.module, "internal/app")) {
				for _, call := range calledSelectors(file.syntax) {
					if slices.Contains(spec.avoids, call.Sel.Name) {
						t.Errorf("%s: app calls raw infrastructure operation %s", file.rel, call.Sel.Name)
					}
				}
			}
		})
	}
}

// calledSelectors lists the selector expressions that are called, such as
// x.Exec in x.Exec(query).
func calledSelectors(syntax *ast.File) []*ast.SelectorExpr {
	var selectors []*ast.SelectorExpr
	ast.Inspect(syntax, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
				selectors = append(selectors, selector)
			}
		}
		return true
	})
	return selectors
}
