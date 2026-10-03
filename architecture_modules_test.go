package agenticstream

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// moduleMapPath is the public module map that architecture-bar rule A9
// requires to list every production package.
const moduleMapPath = "documentation/architecture/repository-map.md"

// TestEveryPackageDocumentsItsResponsibility enforces architecture-bar rule
// A9: each production package names its responsibility in a package comment.
func TestEveryPackageDocumentsItsResponsibility(t *testing.T) {
	t.Parallel()

	documented := documentedPackages(t, repoRoot(t))
	for _, pkg := range slices.Sorted(maps.Keys(documented)) {
		if !documented[pkg] {
			t.Errorf("%s has no package comment; state its business or infrastructure responsibility in doc.go", pkg)
		}
	}
}

// TestModuleMapListsEveryPackage enforces architecture-bar rule A9: the public
// module map names every production package.
func TestModuleMapListsEveryPackage(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	moduleMap := readModuleMap(t, root)
	for _, pkg := range slices.Sorted(maps.Keys(documentedPackages(t, root))) {
		if !strings.Contains(moduleMap, "`"+pkg+"/`") {
			t.Errorf("%s is missing from %s", pkg, moduleMapPath)
		}
	}
}

// documentedPackages maps each production package directory to whether any
// hand-written file carries a "Package <name>" comment. Generated-only
// packages are exempt from the comment rule and reported as documented.
func documentedPackages(t *testing.T, root string) map[string]bool {
	t.Helper()

	documented := map[string]bool{}
	for _, file := range productionGoFiles(t, root) {
		pkg := path.Dir(file.rel)
		if pkg == "." || strings.HasPrefix(pkg, "examples/") {
			continue
		}
		documented[pkg] = documented[pkg] || fileDocumentsPackage(t, file)
	}
	return documented
}

// fileDocumentsPackage reports whether the file is generated or carries the
// package comment.
func fileDocumentsPackage(t *testing.T, file goFile) bool {
	t.Helper()

	if _, generated := countLines(t, file.abs); generated {
		return true
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), file.abs, nil, parser.PackageClauseOnly|parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", file.rel, err)
	}
	return parsed.Doc != nil && strings.HasPrefix(parsed.Doc.Text(), "Package "+parsed.Name.Name+" ")
}

func readModuleMap(t *testing.T, root string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(root, moduleMapPath))
	if err != nil {
		t.Fatalf("read module map: %v", err)
	}
	return string(content)
}

// executorTransportImports are the worker protocol and transport packages
// that only a concrete executor under internal/executor may import.
var executorTransportImports = []string{"google.golang.org/grpc", "google.golang.org/protobuf", "/internal/worker", "/proto/"}

// TestEpisodeLifecycleImportsNoExecutorTransport enforces architecture-bar
// rule A8: the episode lifecycle depends on the Executor port only, so it
// classifies failures without knowing how a concrete executor communicates.
func TestEpisodeLifecycleImportsNoExecutorTransport(t *testing.T) {
	t.Parallel()

	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/episodes" {
			continue
		}
		for _, imported := range fileImports(t, file) {
			if isExecutorTransport(imported) {
				t.Errorf("%s imports executor transport %s; move it to an executor under internal/executor", file.rel, imported)
			}
		}
	}
}

func fileImports(t *testing.T, file goFile) []string {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), file.abs, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", file.rel, err)
	}
	imports := make([]string, 0, len(parsed.Imports))
	for _, spec := range parsed.Imports {
		imports = append(imports, strings.Trim(spec.Path.Value, `"`))
	}
	return imports
}

func isExecutorTransport(imported string) bool {
	return slices.ContainsFunc(executorTransportImports, func(transport string) bool {
		return strings.Contains(imported, transport)
	})
}

// compositionRoots wire modules and drive loops; they own no business rule.
var compositionRoots = []string{"internal/runtime", "cmd/agentic-stream"}

// sqlStatement matches a string literal that is a SQL statement.
var sqlStatement = regexp.MustCompile(`(?i)^\s*(SELECT|INSERT|UPDATE|DELETE|REPLACE|WITH)\s`)

// TestCompositionRootsContainNoSQL enforces architecture-bar rule A10: reads
// and writes belong to the module that owns the data, never to composition.
func TestCompositionRootsContainNoSQL(t *testing.T) {
	t.Parallel()

	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if !slices.Contains(compositionRoots, path.Dir(file.rel)) {
			continue
		}
		for _, literal := range stringLiterals(t, file) {
			if sqlStatement.MatchString(literal) {
				t.Errorf("%s contains SQL %q; call the owning module instead", file.rel, firstLine(literal))
			}
		}
	}
}

func stringLiterals(t *testing.T, file goFile) []string {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), file.abs, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file.rel, err)
	}
	var literals []string
	ast.Inspect(parsed, func(node ast.Node) bool {
		if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING {
			if value, err := strconv.Unquote(literal.Value); err == nil {
				literals = append(literals, value)
			}
		}
		return true
	})
	return literals
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}
