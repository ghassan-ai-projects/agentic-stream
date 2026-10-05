package agenticstream

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"slices"
	"strconv"
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
var impureDomainModulePackages = []string{"internal/storage", "internal/control", "internal/clock"}

// clockReads are time functions that read the wall clock.
var clockReads = []string{"Now", "Since", "Until"}

// TestDomainPackagesArePure enforces that a module's domain layer performs no
// I/O and reads no clock: time arrives as a parameter.
func TestDomainPackagesArePure(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	module := readModulePath(t, filepath.Join(root, "go.mod"))
	for _, file := range layerFiles(t, root, "domain") {
		parsed := parseGoFile(t, file)
		for _, imported := range importPaths(t, file, parsed) {
			if impureDomainImport(module, imported) {
				t.Errorf("%s: domain layer imports %s", file.rel, imported)
			}
		}
		for _, call := range clockCalls(parsed) {
			t.Errorf("%s: domain layer reads the clock with time.%s; take time as a parameter", file.rel, call)
		}
	}
}

// databaseImports are packages only a store layer may use.
var databaseImports = []string{"database/sql", "internal/storage"}

// TestApplicationLayersDoNotTouchTheDatabase enforces that a module's use
// cases reach persistence only through its store's units of work.
func TestApplicationLayersDoNotTouchTheDatabase(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	module := readModulePath(t, filepath.Join(root, "go.mod"))
	for _, file := range layerFiles(t, root, "app") {
		for _, imported := range importPaths(t, file, parseGoFile(t, file)) {
			if slices.Contains(databaseImports, strings.TrimPrefix(imported, module+"/")) {
				t.Errorf("%s: application layer imports %s; use the store's unit of work", file.rel, imported)
			}
		}
	}
}

// TestModuleSQLStaysInStore enforces that a module with a store layer keeps
// every SQL statement there.
func TestModuleSQLStaysInStore(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	modules := modulesWithStore(t, root)
	for _, file := range productionGoFiles(t, root) {
		pkg := path.Dir(file.rel)
		module, ok := owningLayeredModule(pkg, modules)
		if !ok || pkg == module+"/internal/store" {
			continue
		}
		for _, literal := range stringLiterals(t, file) {
			if sqlStatement.MatchString(literal) {
				t.Errorf("%s: SQL %q outside %s/internal/store", file.rel, firstLine(literal), module)
			}
		}
	}
}

// layerFiles lists production files of every internal/<module>/internal/<layer>.
func layerFiles(t *testing.T, root, layer string) []goFile {
	t.Helper()
	var files []goFile
	for _, file := range productionGoFiles(t, root) {
		pkg := path.Dir(file.rel)
		if path.Base(pkg) == layer && path.Base(path.Dir(pkg)) == "internal" {
			files = append(files, file)
		}
	}
	return files
}

func modulesWithStore(t *testing.T, root string) []string {
	t.Helper()
	var modules []string
	for _, file := range layerFiles(t, root, "store") {
		module := path.Dir(path.Dir(path.Dir(file.rel)))
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

func parseGoFile(t *testing.T, file goFile) *ast.File {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file.abs, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file.rel, err)
	}
	return parsed
}

func importPaths(t *testing.T, file goFile, parsed *ast.File) []string {
	t.Helper()
	var paths []string
	for _, spec := range parsed.Imports {
		value, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("unquote import in %s: %v", file.rel, err)
		}
		paths = append(paths, value)
	}
	return paths
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
