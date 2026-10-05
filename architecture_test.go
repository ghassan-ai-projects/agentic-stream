package agenticstream

import (
	"bufio"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// maxProductionFileLines is quality-bar rule Q3: production files stay under
// 300 lines. See .agents/context/quality-bar.md.
const maxProductionFileLines = 299

// foundationPackages sit at the bottom of the dependency graph. They may import
// each other but never a domain plane (quality-bar rule Q5).
var foundationPackages = []string{
	"internal/canonicaljson",
	"internal/clock",
	"internal/contractsv1",
	"internal/costcontrol",
	"internal/duration",
	"internal/eventschema",
	"internal/ids",
	"internal/interlock",
	"internal/storage",
	"internal/telemetry",
	"migrations",
	"proto/agenticstream/runtime/v1",
}

// forbiddenImports names edges that would break a product invariant rather
// than just tidiness: models propose but never reach the policy or action
// plane, and replay never performs external effects.
var forbiddenImports = map[string][]string{
	"internal/cognition":            {"internal/policy", "internal/actions"},
	"internal/decisions":            {"internal/policy", "internal/actions"},
	"internal/episodes":             {"internal/policy", "internal/actions", "internal/worker", "proto/agenticstream/runtime/v1", "internal/evidence"},
	"internal/evidence":             {"internal/policy", "internal/actions"},
	"internal/executor/native":      {"internal/policy", "internal/actions"},
	"internal/executor/remote":      {"internal/policy", "internal/actions"},
	"internal/worker":               {"internal/policy", "internal/actions"},
	"internal/actions":              {"internal/device", "internal/watch"},
	"internal/watch":                {"internal/actions", "internal/policy", "internal/episodes", "internal/cognition"},
	"internal/replay":               {"internal/actions", "internal/runtime"},
	"internal/executor/conformance": {"internal/actions"},
}

// allowedImports is the reviewed internal dependency graph. A new edge must
// follow the data flow in .agents/context/architecture.md and be added here in
// the same change.
var allowedImports = map[string][]string{
	"cmd/agentic-stream":                 {"internal/actionport", "internal/api", "internal/authority", "internal/contractsv1", "internal/control", "internal/device", "internal/evidence", "internal/ids", "internal/replay", "internal/runartifact", "internal/runtime", "internal/spec", "internal/storage", "internal/telemetry"},
	"internal/actionport":                {},
	"internal/actions":                   {"internal/actionport", "internal/authority", "internal/canonicaljson", "internal/clock", "internal/contractsv1", "internal/control", "internal/ids", "internal/interlock", "internal/notify", "internal/storage", "internal/telemetry"},
	"internal/admission":                 {"internal/clock", "internal/cognition", "internal/control", "internal/costcontrol", "internal/episodeledger", "internal/episodes", "internal/scheduleledger", "internal/storage"},
	"internal/api":                       {"internal/canonicaljson", "internal/control", "internal/notify", "internal/storage"},
	"internal/approvalledger":            {"internal/clock", "internal/contractsv1", "internal/notify"},
	"internal/authority":                 {"internal/authority/internal/domain", "internal/authority/internal/store", "internal/clock", "internal/control", "internal/storage"},
	"internal/authority/internal/domain": {"internal/canonicaljson"},
	"internal/authority/internal/store":  {"internal/authority/internal/domain", "internal/canonicaljson"},
	"internal/canonicaljson":             {},
	"internal/clock":                     {},
	"internal/cognition":                 {"internal/approvalledger", "internal/canonicaljson", "internal/clock", "internal/contractsv1", "internal/duration", "internal/episodeledger", "internal/ids", "internal/notify", "internal/scheduleledger", "internal/situations", "internal/spec", "internal/storage"},
	"internal/contractsv1":               {"internal/canonicaljson"},
	"internal/control":                   {"internal/actionport", "internal/costcontrol", "internal/episodeledger", "internal/interlock", "internal/storage"},
	"internal/costcontrol":               {},
	"internal/decisions":                 {"internal/canonicaljson", "internal/contractsv1"},
	"internal/device":                    {"internal/actionport", "internal/authority", "internal/canonicaljson", "internal/contractsv1", "internal/control", "internal/telemetry"},
	"internal/duration":                  {},
	"internal/engine":                    {"internal/canonicaljson", "internal/clock", "internal/cognition", "internal/contractsv1", "internal/control", "internal/duration", "internal/eventlog", "internal/ids", "internal/operators", "internal/situations", "internal/spec", "internal/storage"},
	"internal/episodeledger":             {"internal/costcontrol"},
	"internal/episodes":                  {"internal/canonicaljson", "internal/clock", "internal/contractsv1", "internal/control", "internal/costcontrol", "internal/decisions", "internal/duration", "internal/episodeledger", "internal/ids", "internal/qualification", "internal/scheduleledger", "internal/spec", "internal/storage", "internal/telemetry"},
	"internal/eventlog":                  {"internal/clock", "internal/contractsv1", "internal/storage"},
	"internal/eventschema":               {},
	"internal/evidence":                  {"internal/contractsv1", "internal/control", "internal/eventlog", "internal/storage", "proto/agenticstream/runtime/v1"},
	"internal/executor/conformance":      {"internal/canonicaljson", "internal/episodeledger", "internal/episodes", "internal/spec"},
	"internal/executor/native":           {"internal/canonicaljson", "internal/episodeledger", "internal/episodes", "internal/eventlog", "internal/storage"},
	"internal/executor/remote":           {"internal/canonicaljson", "internal/episodeledger", "internal/episodes", "internal/evidence", "internal/spec", "internal/telemetry", "internal/worker", "proto/agenticstream/runtime/v1"},
	"internal/ids":                       {},
	"internal/ingress":                   {"internal/clock", "internal/contractsv1", "internal/eventlog", "internal/ids", "internal/storage", "internal/telemetry"},
	"internal/interlock":                 {},
	"internal/notify":                    {"internal/canonicaljson", "internal/contractsv1", "internal/ids", "internal/notifycontract", "internal/storage"},
	"internal/notifycontract":            {"internal/contractsv1"},
	"internal/operators":                 {"internal/contractsv1", "internal/duration", "internal/eventschema", "internal/ids", "internal/spec"},
	"internal/policy":                    {"internal/approvalledger", "internal/canonicaljson", "internal/contractsv1", "internal/control", "internal/ids", "internal/interlock", "internal/notify", "internal/qualification"},
	"internal/qualification":             {"internal/storage"},
	"internal/replay":                    {"internal/canonicaljson", "internal/clock", "internal/contractsv1", "internal/decisions", "internal/engine", "internal/episodes", "internal/eventlog", "internal/ids", "internal/ingress", "internal/policy", "internal/qualification", "internal/spec", "internal/storage"},
	"internal/runartifact":               {"internal/canonicaljson", "internal/policy", "internal/soak", "internal/storage"},
	"internal/runtime":                   {"internal/actionport", "internal/actions", "internal/admission", "internal/clock", "internal/contractsv1", "internal/control", "internal/costcontrol", "internal/device", "internal/engine", "internal/episodeledger", "internal/episodes", "internal/eventlog", "internal/evidence", "internal/executor/native", "internal/executor/remote", "internal/ids", "internal/ingress", "internal/interlock", "internal/policy", "internal/qualification", "internal/spec", "internal/storage", "internal/telemetry", "internal/watch", "internal/worker", "proto/agenticstream/runtime/v1"},
	"internal/scheduleledger":            {},
	"internal/situations":                {"internal/canonicaljson", "internal/contractsv1", "internal/duration", "internal/ids", "internal/operators", "internal/spec"},
	"internal/soak":                      {"internal/authority", "internal/canonicaljson", "internal/storage"},
	"internal/spec":                      {"internal/canonicaljson", "internal/eventschema", "internal/storage"},
	"internal/storage":                   {"migrations"},
	"internal/telemetry":                 {},
	"internal/watch":                     {"internal/actionport", "internal/clock", "internal/control", "internal/interlock", "internal/storage"},
	"internal/worker":                    {"internal/contractsv1", "proto/agenticstream/runtime/v1"},
	"migrations":                         {},
	"proto/agenticstream/runtime/v1":     {},
}

// TestPackageLayering enforces quality-bar rule Q5: production imports between
// this module's packages follow the reviewed dependency graph.
func TestPackageLayering(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	module := readModulePath(t, filepath.Join(root, "go.mod"))
	graph := productionImportGraph(t, root, module)

	foundation := make(map[string]bool, len(foundationPackages))
	for _, pkg := range foundationPackages {
		foundation[pkg] = true
	}

	for _, pkg := range slices.Sorted(maps.Keys(graph)) {
		allowed, known := allowedImports[pkg]
		if !known {
			t.Errorf("package %s is not declared in allowedImports; add it with its reviewed imports", pkg)
			continue
		}
		for _, imported := range graph[pkg] {
			switch {
			case strings.HasPrefix(imported, "cmd/"):
				t.Errorf("%s imports %s: nothing may import a command package", pkg, imported)
			case foundation[pkg] && !foundation[imported]:
				t.Errorf("%s is a foundation package and must not import domain package %s", pkg, imported)
			case slices.Contains(forbiddenImports[pkg], imported):
				t.Errorf("%s must never import %s: it would break a product invariant", pkg, imported)
			case !slices.Contains(allowed, imported):
				t.Errorf("%s imports %s, which is not in allowedImports; follow the data flow in .agents/context/architecture.md", pkg, imported)
			}
		}
	}
	for _, pkg := range slices.Sorted(maps.Keys(allowedImports)) {
		if _, exists := graph[pkg]; !exists {
			t.Errorf("allowedImports declares %s, which has no production Go files; remove the stale entry", pkg)
		}
	}
}

// TestAllowedImportsHaveNoStaleEdges keeps the reviewed graph exact: an
// approved edge that no production file uses must be removed, so the
// allowlist documents real dependencies rather than permissions.
func TestAllowedImportsHaveNoStaleEdges(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	graph := productionImportGraph(t, root, readModulePath(t, filepath.Join(root, "go.mod")))
	for _, pkg := range slices.Sorted(maps.Keys(allowedImports)) {
		for _, allowed := range allowedImports[pkg] {
			if !slices.Contains(graph[pkg], allowed) {
				t.Errorf("allowedImports approves %s -> %s, which no production file uses; remove the stale edge", pkg, allowed)
			}
		}
	}
}

// TestProductionFileSize enforces quality-bar rule Q3.
func TestProductionFileSize(t *testing.T) {
	t.Parallel()

	for _, file := range productionGoFiles(t, repoRoot(t)) {
		lines, generated := countLines(t, file.abs)
		if !generated && lines > maxProductionFileLines {
			t.Errorf("%s has %d lines; split it by responsibility to under %d", file.rel, lines, maxProductionFileLines+1)
		}
	}
}

type goFile struct {
	abs string
	rel string
}

// productionGoFiles lists non-test Go files that belong to this module.
func productionGoFiles(t *testing.T, root string) []goFile {
	t.Helper()

	var files []goFile
	err := filepath.WalkDir(root, func(abs string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if abs != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor" || name == "tmp" || name == "bin") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(abs, ".go") || strings.HasSuffix(abs, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return err
		}
		files = append(files, goFile{abs: abs, rel: filepath.ToSlash(rel)})
		return nil
	})
	if err != nil {
		t.Fatalf("walk repository: %v", err)
	}
	return files
}

// productionImportGraph maps each package directory, relative to the module
// root, to the sorted module-internal packages its production files import.
func productionImportGraph(t *testing.T, root string, module string) map[string][]string {
	t.Helper()

	graph := map[string][]string{}
	fset := token.NewFileSet()
	for _, file := range productionGoFiles(t, root) {
		pkg := path.Dir(file.rel)
		if pkg == "." || strings.HasPrefix(pkg, "examples/") {
			continue
		}
		parsed, err := parser.ParseFile(fset, file.abs, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", file.rel, err)
		}
		imports := graph[pkg]
		for _, spec := range parsed.Imports {
			value, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", file.rel, err)
			}
			if rest, ok := strings.CutPrefix(value, module+"/"); ok && !slices.Contains(imports, rest) {
				imports = append(imports, rest)
			}
		}
		slices.Sort(imports)
		graph[pkg] = imports
	}
	return graph
}

// countLines returns the file's line count and whether it carries the standard
// "Code generated ... DO NOT EDIT." marker.
func countLines(t *testing.T, file string) (int, bool) {
	t.Helper()

	handle, err := os.Open(file)
	if err != nil {
		t.Fatalf("open %s: %v", file, err)
	}
	defer func() { _ = handle.Close() }()

	lines := 0
	generated := false
	scanner := bufio.NewScanner(handle)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines++
		text := scanner.Text()
		if strings.HasPrefix(text, "// Code generated ") && strings.HasSuffix(text, " DO NOT EDIT.") {
			generated = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	return lines, generated
}
