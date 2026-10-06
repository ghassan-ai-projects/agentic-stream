package agenticstream

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// packageLayers are reviewed dependency levels, not automatically computed
// depths. Adding an acyclic edge still fails if it crosses upward or sideways.
var packageLayers = map[string]int{
	"internal/actionport":                         0,
	"internal/approvalledger/internal/domain":     0,
	"internal/canonicaljson/internal/domain":      0,
	"internal/episodeledger/internal/domain":      0,
	"internal/interlock":                          0,
	"internal/sources":                            0,
	"internal/telemetry/internal/domain":          0,
	"migrations":                                  0,
	"proto/agenticstream/runtime/v1":              0,
	"internal/telemetry/internal/transport":       1,
	"internal/telemetry":                          2,
	"internal/approvalledger/internal/store":      3,
	"internal/canonicaljson":                      3,
	"internal/control/internal/domain":            3,
	"internal/episodeledger/internal/store":       3,
	"internal/eventlog/internal/domain":           3,
	"internal/runtime/internal/domain":            3,
	"internal/storage":                            3,
	"internal/approvalledger/internal/app":        4,
	"internal/authority/internal/domain":          4,
	"internal/contractsv1/internal/domain":        4,
	"internal/episodeledger/internal/app":         4,
	"internal/notify/internal/store":              4,
	"internal/runartifact/internal/domain":        4,
	"internal/spec":                               4,
	"internal/watch/internal/domain":              4,
	"internal/contractsv1":                        5,
	"internal/actions/internal/domain":            6,
	"internal/approvalledger":                     6,
	"internal/authority/internal/store":           6,
	"internal/decisions/internal/domain":          6,
	"internal/device/internal/domain":             6,
	"internal/episodeledger":                      6,
	"internal/eventlog/internal/store":            6,
	"internal/evidence/internal/domain":           6,
	"internal/ingress/internal/domain":            6,
	"internal/notify/internal/domain":             6,
	"internal/operators":                          6,
	"internal/policy/internal/domain":             6,
	"internal/runartifact/internal/transport":     6,
	"internal/watch/internal/store":               6,
	"internal/worker/internal/domain":             6,
	"internal/worker/internal/transport":          7,
	"internal/worker":                             8,
	"internal/authority/internal/app":             9,
	"internal/control/internal/store":             9,
	"internal/decisions":                          9,
	"internal/device/internal/wire":               9,
	"internal/eventlog/internal/app":              9,
	"internal/evidence/internal/store":            9,
	"internal/evidence/internal/wire":             9,
	"internal/ingress/internal/store":             9,
	"internal/ingress/internal/transport":         9,
	"internal/notify/internal/app":                9,
	"internal/situations":                         9,
	"internal/watch/internal/app":                 9,
	"internal/cognition/internal/domain":          10,
	"internal/control/internal/app":               10,
	"internal/device/internal/transport":          10,
	"internal/engine/internal/domain":             10,
	"internal/episodes/internal/domain":           10,
	"internal/eventlog":                           10,
	"internal/evidence/internal/app":              10,
	"internal/notify":                             10,
	"internal/replay/internal/domain":             10,
	"internal/watch":                              10,
	"internal/cognition/internal/store":           11,
	"internal/control":                            11,
	"internal/engine/internal/store":              11,
	"internal/evidence/internal/transport":        11,
	"internal/ingress/internal/app":               11,
	"internal/policy/internal/store":              11,
	"internal/api":                                12,
	"internal/authority":                          12,
	"internal/cognition/internal/app":             12,
	"internal/episodes/internal/store":            12,
	"internal/evidence":                           12,
	"internal/ingress":                            12,
	"internal/policy/internal/app":                12,
	"internal/actions/internal/store":             13,
	"internal/cognition":                          13,
	"internal/device/internal/app":                13,
	"internal/episodes/internal/app":              13,
	"internal/policy":                             13,
	"internal/replay/internal/transport":          13,
	"internal/runartifact/internal/store":         13,
	"internal/actions/internal/app":               14,
	"internal/device":                             14,
	"internal/engine/internal/app":                14,
	"internal/episodes":                           14,
	"internal/runartifact/internal/app":           14,
	"internal/actions":                            15,
	"internal/engine":                             15,
	"internal/executor/fixture":                   15,
	"internal/executor/native/internal/domain":    15,
	"internal/executor/remote/internal/domain":    15,
	"internal/runartifact":                        15,
	"internal/runtime/internal/store":             15,
	"internal/testsupport/executorconformance":    15,
	"internal/executor/native/internal/store":     16,
	"internal/executor/native/internal/transport": 16,
	"internal/executor/remote/internal/transport": 16,
	"internal/executor/native/internal/app":       17,
	"internal/executor/remote/internal/app":       17,
	"internal/executor/native":                    18,
	"internal/executor/remote":                    18,
	"internal/replay/internal/store":              19,
	"internal/runtime/internal/transport":         19,
	"internal/replay/internal/app":                20,
	"internal/runtime/internal/app":               20,
	"internal/replay":                             21,
	"internal/runtime/internal/composition":       21,
	"internal/runtime":                            22,
	"cmd/agentic-stream":                          23,
}

func TestImportsOnlyPointToLowerArchitectureLayers(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	module := readModulePath(t, filepath.Join(root, "go.mod"))
	graph := productionImportGraph(t, root, module)
	for pkg, dependencies := range graph {
		layer, ok := packageLayers[pkg]
		if !ok {
			t.Errorf("%s has no reviewed architecture layer", pkg)
			continue
		}
		for _, dependency := range dependencies {
			lower, ok := packageLayers[dependency]
			if !ok {
				t.Errorf("%s depends on unclassified %s", pkg, dependency)
				continue
			}
			if lower >= layer {
				t.Errorf("%s (layer %d) depends upward/sideways on %s (layer %d)", pkg, layer, dependency, lower)
			}
		}
	}
	for pkg := range packageLayers {
		if _, ok := graph[pkg]; !ok {
			t.Errorf("stale architecture layer: %s", pkg)
		}
	}
}

func TestReasoningAndReplayCannotReachEffectImplementations(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	module := readModulePath(t, filepath.Join(root, "go.mod"))
	graph := productionImportGraph(t, root, module)
	reasoning := []string{"internal/cognition", "internal/decisions", "internal/engine", "internal/engine/internal/app", "internal/engine/internal/domain", "internal/engine/internal/store", "internal/episodes", "internal/evidence", "internal/executor/native", "internal/executor/native/internal/app", "internal/executor/native/internal/domain", "internal/executor/native/internal/store", "internal/executor/native/internal/transport", "internal/executor/remote", "internal/executor/remote/internal/app", "internal/executor/remote/internal/domain", "internal/executor/remote/internal/transport", "internal/testsupport/executorconformance", "internal/worker"}
	replayLayers := []string{"internal/replay", "internal/replay/internal/app", "internal/replay/internal/domain", "internal/replay/internal/store", "internal/replay/internal/transport"}
	for _, source := range append(reasoning, replayLayers...) {
		for _, target := range []string{"internal/actions", "internal/actions/internal/app", "internal/actions/internal/store", "internal/device", "internal/device/internal/app", "internal/device/internal/transport", "internal/watch", "internal/runtime", "internal/runtime/internal/app", "internal/runtime/internal/composition", "internal/runtime/internal/store", "internal/runtime/internal/transport", "cmd/agentic-stream"} {
			if path := dependencyPath(graph, source, target); len(path) > 0 {
				t.Errorf("effect implementation reachable: %s", strings.Join(path, " -> "))
			}
		}
	}
	for _, source := range reasoning {
		for _, target := range []string{"internal/policy", "internal/policy/internal/app", "internal/policy/internal/store", "internal/policy/internal/domain"} {
			if path := dependencyPath(graph, source, target); len(path) > 0 {
				t.Errorf("reasoning reaches policy: %s", strings.Join(path, " -> "))
			}
		}
	}
	for _, adapter := range []string{"internal/device", "internal/device/internal/app"} {
		for _, target := range []string{"internal/actions", "internal/policy", "internal/episodes", "internal/runtime"} {
			if path := dependencyPath(graph, adapter, target); len(path) > 0 {
				t.Errorf("device adapter reaches upstream service: %s", strings.Join(path, " -> "))
			}
		}
	}
}

func dependencyPath(graph map[string][]string, source, target string) []string {
	queue := [][]string{{source}}
	visited := map[string]bool{source: true}
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		node := path[len(path)-1]
		if node == target {
			return path
		}
		for _, next := range graph[node] {
			if !visited[next] {
				visited[next] = true
				queue = append(queue, append(slices.Clone(path), next))
			}
		}
	}
	return nil
}

func TestDependencyReachabilityIncludesIndirectAndCyclicPaths(t *testing.T) {
	t.Parallel()
	graph := map[string][]string{"reasoning": {"ledger"}, "ledger": {"reasoning", "adapter"}}
	if path := dependencyPath(graph, "reasoning", "adapter"); !slices.Equal(path, []string{"reasoning", "ledger", "adapter"}) {
		t.Fatalf("indirect path=%v", path)
	}
	if path := dependencyPath(graph, "reasoning", "unreachable"); len(path) != 0 {
		t.Fatalf("cycle produced false path=%v", path)
	}
}

func TestContractPackagesExcludePersistenceAndTransport(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	for _, file := range productionGoFiles(t, root) {
		pkg := filepath.ToSlash(filepath.Dir(file.rel))
		if pkg != "internal/actionport" && pkg != "internal/contractsv1" {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file.abs, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, dependency := range parsed.Imports {
			name, err := strconv.Unquote(dependency.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if name == "database/sql" || name == "net" || strings.HasPrefix(name, "net/") || strings.HasSuffix(name, "/internal/storage") {
				t.Errorf("contract package %s imports implementation %s", pkg, name)
			}
		}
	}
}

func TestFinalAuthorizationIsConstructedOnlyByLowerControlCapability(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	module := readModulePath(t, filepath.Join(root, "go.mod"))
	for _, file := range productionGoFiles(t, root) {
		parsed, err := parser.ParseFile(token.NewFileSet(), file.abs, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		aliases := map[string]bool{}
		for _, dependency := range parsed.Imports {
			name, _ := strconv.Unquote(dependency.Path.Value)
			if name != module+"/internal/actionport" {
				continue
			}
			alias := "actionport"
			if dependency.Name != nil {
				alias = dependency.Name.Name
			}
			aliases[alias] = true
		}
		pkg := filepath.ToSlash(filepath.Dir(file.rel))
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			typ, ok := literal.Type.(*ast.SelectorExpr)
			if !ok || typ.Sel.Name != "Authorization" {
				return true
			}
			alias, ok := typ.X.(*ast.Ident)
			if ok && aliases[alias.Name] && pkg != "internal/control" {
				t.Errorf("%s constructs an upstream authorization callback; use control.NewDispatchAuthorization", file.rel)
			}
			return true
		})
	}
}
