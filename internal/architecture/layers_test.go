package architecture

import (
	"go/ast"
	"path"
	"slices"
	"strings"
	"testing"
)

// The reference module pattern (docs/authority-reference-module-2026-10-05)
// splits a module into internal/<module>/internal/domain (pure rules) and
// internal/<module>/internal/store (all SQL). These tests keep any module that
// adopts the pattern honest.

// impureDomainImports are standard-library and module packages a domain layer
// must not use: storage, transactions, transport, process state and logging.
var impureDomainImports = []string{
	"database/sql", "io/fs", "log", "log/slog", "net", "net/http", "os", "os/exec", "syscall",
}

// impureDomainModulePackages are module packages a domain layer must not use.
var impureDomainModulePackages = []string{"internal/storage", "internal/control"}

// clockReads are time functions that read the wall clock.
var clockReads = []string{"Now", "Since", "Until"}

// TestDomainPackagesArePure enforces that a module's domain layer performs no
// I/O and reads no clock: time arrives as a parameter.
func TestDomainPackagesArePure(t *testing.T) {
	t.Parallel()

	repo := loadRepository(t)
	for _, file := range repo.layerFiles("domain") {
		for _, imported := range file.imports {
			if impureDomainImport(repo.module, imported) {
				t.Errorf("%s: domain layer imports %s", file.rel, imported)
			}
		}
		for _, call := range clockCalls(file.syntax) {
			t.Errorf("%s: domain layer reads the clock with time.%s; take time as a parameter", file.rel, call)
		}
	}
}

// infrastructureImports are packages only a store or adapter layer may use:
// the database and the network.
var infrastructureImports = []string{"database/sql", "internal/storage", "net", "net/http"}

// TestApplicationLayersDoNotTouchInfrastructure enforces that a module's use
// cases reach the database and the network only through its store or adapter
// layers.
func TestApplicationLayersDoNotTouchInfrastructure(t *testing.T) {
	t.Parallel()

	repo := loadRepository(t)
	for _, file := range repo.layerFiles("app") {
		for _, imported := range file.imports {
			if slices.Contains(infrastructureImports, strings.TrimPrefix(imported, repo.module+"/")) {
				t.Errorf("%s: application layer imports %s; go through the store or an adapter layer", file.rel, imported)
			}
		}
	}
}

// TestModuleSQLStaysInStore enforces that a module with a store layer keeps
// every SQL statement there.
func TestModuleSQLStaysInStore(t *testing.T) {
	t.Parallel()

	repo := loadRepository(t)
	modules := modulesWithStore(repo)
	for _, file := range repo.production {
		module, ok := owningLayeredModule(file.dir, modules)
		if !ok || file.dir == module+"/internal/store" {
			continue
		}
		for _, literal := range stringLiterals(file) {
			if sqlStatement.MatchString(literal) {
				t.Errorf("%s: SQL %q outside %s/internal/store", file.rel, firstLine(literal), module)
			}
		}
	}
}

// compositionRoots wire modules and drive loops; they own no business rule.
var compositionRoots = []string{"internal/runtime", "cmd/agentic-stream"}

// TestCompositionRootsContainNoSQL enforces architecture-bar rule A10: reads
// and writes belong to the module that owns the data, never to composition.
func TestCompositionRootsContainNoSQL(t *testing.T) {
	t.Parallel()

	repo := loadRepository(t)
	for _, file := range repo.production {
		if !slices.Contains(compositionRoots, file.dir) {
			continue
		}
		for _, literal := range stringLiterals(file) {
			if sqlStatement.MatchString(literal) {
				t.Errorf("%s contains SQL %q; call the owning module instead", file.rel, firstLine(literal))
			}
		}
	}
}

func modulesWithStore(repo *repository) []string {
	var modules []string
	for _, file := range repo.layerFiles("store") {
		module := path.Dir(path.Dir(file.dir))
		if !slices.Contains(modules, module) {
			modules = append(modules, module)
		}
	}
	return modules
}

func owningLayeredModule(pkg string, modules []string) (string, bool) {
	for _, module := range modules {
		if pkg == module || strings.HasPrefix(pkg, module+"/") {
			return module, true
		}
	}
	return "", false
}

func impureDomainImport(module, imported string) bool {
	if slices.Contains(impureDomainImports, imported) {
		return true
	}
	rest, ok := strings.CutPrefix(imported, module+"/")
	return ok && (slices.Contains(impureDomainModulePackages, rest) || path.Base(rest) == "store")
}

// clockCalls finds time.Now, time.Since and time.Until selectors.
func clockCalls(parsed *ast.File) []string {
	var calls []string
	ast.Inspect(parsed, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if ident, ok := selector.X.(*ast.Ident); ok && ident.Name == "time" && slices.Contains(clockReads, selector.Sel.Name) {
			calls = append(calls, selector.Sel.Name)
		}
		return true
	})
	return calls
}

// nondeterministicSources are the symbols of internal/sources that read the wall
// clock or a random generator. Deterministic layers may share the package's
// vocabulary (identity prefixes and the deterministic generator) but never these.
var nondeterministicSources = []string{"Clock", "Timer", "Virtual", "Physical", "NewVirtual", "Quality", "Random"}

// TestDeterministicLayersDoNotUseTimeOrRandomSources enforces that domain layers
// and the replay store take time and randomness as parameters instead of
// reaching for a clock or a random generator.
func TestDeterministicLayersDoNotUseTimeOrRandomSources(t *testing.T) {
	t.Parallel()

	for _, file := range loadRepository(t).production {
		if path.Base(file.dir) != "domain" && file.dir != "internal/replay/internal/store" {
			continue
		}
		for _, symbol := range sourceSelectors(file.syntax) {
			if slices.Contains(nondeterministicSources, symbol) {
				t.Errorf("%s: deterministic layer uses sources.%s; take it as a parameter", file.rel, symbol)
			}
		}
	}
}

// sourceSelectors lists the names selected from the sources package.
func sourceSelectors(parsed *ast.File) []string {
	var names []string
	ast.Inspect(parsed, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if ident, ok := selector.X.(*ast.Ident); ok && ident.Name == "sources" {
			names = append(names, selector.Sel.Name)
		}
		return true
	})
	return names
}
