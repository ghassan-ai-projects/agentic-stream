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
	"internal/api/internal/domain":                4,
	"internal/approvalledger/internal/app":        4,
	"internal/authority/internal/domain":          4,
	"internal/contractsv1/internal/domain":        4,
	"internal/episodeledger/internal/app":         4,
	"internal/notify/internal/store":              4,
	"internal/runartifact/internal/domain":        4,
	"internal/spec/internal/domain":               4,
	"internal/watch/internal/domain":              4,
	"internal/spec/internal/app":                  5,
	"internal/spec/internal/store":                5,
	"internal/spec":                               6,
	"internal/contractsv1":                        7,
	"internal/actions/internal/domain":            8,
	"internal/approvalledger":                     8,
	"internal/authority/internal/store":           8,
	"internal/decisions/internal/domain":          8,
	"internal/device/internal/domain":             8,
	"internal/episodeledger":                      8,
	"internal/eventlog/internal/store":            8,
	"internal/evidence/internal/domain":           8,
	"internal/ingress/internal/domain":            8,
	"internal/notify/internal/domain":             8,
	"internal/operators/internal/domain":          8,
	"internal/policy/internal/domain":             8,
	"internal/runartifact/internal/transport":     8,
	"internal/watch/internal/store":               8,
	"internal/worker/internal/domain":             8,
	"internal/operators":                          9,
	"internal/worker/internal/transport":          10,
	"internal/worker":                             11,
	"internal/authority/internal/app":             12,
	"internal/control/internal/store":             12,
	"internal/decisions":                          12,
	"internal/device/internal/wire":               12,
	"internal/eventlog/internal/app":              12,
	"internal/evidence/internal/store":            12,
	"internal/evidence/internal/wire":             12,
	"internal/ingress/internal/store":             12,
	"internal/ingress/internal/transport":         12,
	"internal/notify/internal/app":                12,
	"internal/situations/internal/domain":         12,
	"internal/watch/internal/app":                 12,
	"internal/situations":                         13,
	"internal/cognition/internal/domain":          14,
	"internal/control/internal/app":               14,
	"internal/device/internal/transport":          14,
	"internal/engine/internal/domain":             14,
	"internal/episodes/internal/domain":           14,
	"internal/eventlog":                           14,
	"internal/evidence/internal/app":              14,
	"internal/notify":                             14,
	"internal/replay/internal/domain":             14,
	"internal/watch":                              14,
	"internal/cognition/internal/store":           15,
	"internal/control":                            15,
	"internal/engine/internal/store":              15,
	"internal/evidence/internal/transport":        15,
	"internal/ingress/internal/app":               15,
	"internal/policy/internal/store":              15,
	"internal/api/internal/transport":             16,
	"internal/authority":                          16,
	"internal/cognition/internal/app":             16,
	"internal/episodes/internal/store":            16,
	"internal/evidence":                           16,
	"internal/ingress":                            16,
	"internal/policy/internal/app":                16,
	"internal/api":                                17,
	"internal/actions/internal/store":             18,
	"internal/cognition":                          18,
	"internal/device/internal/app":                18,
	"internal/episodes/internal/app":              18,
	"internal/policy":                             18,
	"internal/replay/internal/transport":          18,
	"internal/runartifact/internal/store":         18,
	"internal/actions/internal/app":               19,
	"internal/device":                             19,
	"internal/engine/internal/app":                19,
	"internal/episodes":                           19,
	"internal/runartifact/internal/app":           19,
	"internal/actions":                            20,
	"internal/engine":                             20,
	"internal/executor/fixture":                   20,
	"internal/executor/native/internal/domain":    20,
	"internal/executor/remote/internal/domain":    20,
	"internal/runartifact":                        20,
	"internal/runtime/internal/store":             20,
	"internal/testsupport/executorconformance":    20,
	"internal/executor/native/internal/store":     21,
	"internal/executor/native/internal/transport": 21,
	"internal/executor/remote/internal/transport": 21,
	"internal/executor/native/internal/app":       22,
	"internal/executor/remote/internal/app":       22,
	"internal/executor/native":                    23,
	"internal/executor/remote":                    23,
	"internal/replay/internal/store":              24,
	"internal/runtime/internal/transport":         24,
	"internal/replay/internal/app":                25,
	"internal/runtime/internal/app":               25,
	"internal/replay":                             26,
	"internal/runtime/internal/composition":       26,
	"internal/runtime":                            27,
	"cmd/agentic-stream":                          28,
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
