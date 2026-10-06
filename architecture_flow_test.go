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
	"internal/runtime/internal/domain": 1, "internal/runtime/internal/transport": 14, "internal/runtime/internal/store": 10, "internal/runtime/internal/app": 15, "internal/runtime/internal/composition": 16,
	"internal/actionport": 0, "internal/canonicaljson": 1, "internal/canonicaljson/internal/domain": 0, "internal/sources": 0,
	"internal/interlock": 0, "internal/telemetry": 0,
	"migrations": 0, "proto/agenticstream/runtime/v1": 0,
	"internal/authority/internal/domain": 2, "internal/contractsv1": 2, "internal/storage": 1,
	"internal/policy/internal/domain": 3, "internal/decisions": 4, "internal/decisions/internal/domain": 3, "internal/device/internal/domain": 3, "internal/device/internal/wire": 4, "internal/episodeledger": 3, "internal/episodeledger/internal/domain": 0, "internal/episodeledger/internal/store": 1, "internal/episodeledger/internal/app": 2, "internal/eventlog": 5, "internal/evidence": 7, "internal/evidence/internal/domain": 3, "internal/evidence/internal/wire": 4, "internal/evidence/internal/store": 4, "internal/evidence/internal/app": 5, "internal/evidence/internal/transport": 6,
	"internal/authority/internal/store": 3, "internal/spec": 2, "internal/worker": 3,
	"internal/authority/internal/app": 4, "internal/control": 6, "internal/control/internal/domain": 1, "internal/control/internal/store": 4, "internal/control/internal/app": 5, "internal/device/internal/transport": 5, "internal/eventlog/internal/domain": 1, "internal/eventlog/internal/store": 3, "internal/eventlog/internal/app": 4, "internal/ingress": 7, "internal/ingress/internal/domain": 3, "internal/ingress/internal/store": 4, "internal/ingress/internal/transport": 4, "internal/ingress/internal/app": 6, "internal/notify": 5, "internal/notify/internal/domain": 3, "internal/notify/internal/store": 2, "internal/notify/internal/app": 4, "internal/operators": 3,
	"internal/api": 7, "internal/approvalledger": 3, "internal/approvalledger/internal/domain": 0, "internal/approvalledger/internal/store": 1, "internal/approvalledger/internal/app": 2, "internal/watch": 5, "internal/watch/internal/domain": 2, "internal/watch/internal/store": 3, "internal/watch/internal/app": 4, "internal/authority": 7, "internal/situations": 4,
	"internal/actions": 10, "internal/actions/internal/domain": 3, "internal/actions/internal/store": 8, "internal/actions/internal/app": 9, "internal/cognition": 8, "internal/cognition/internal/domain": 5, "internal/cognition/internal/store": 6, "internal/cognition/internal/app": 7, "internal/device/internal/app": 8, "internal/episodes": 9, "internal/episodes/internal/domain": 5, "internal/episodes/internal/store": 7, "internal/episodes/internal/app": 8, "internal/policy/internal/store": 6, "internal/policy/internal/app": 7, "internal/policy": 8,
	"internal/device": 9, "internal/engine": 10, "internal/engine/internal/domain": 5, "internal/engine/internal/store": 6, "internal/engine/internal/app": 9, "internal/testsupport/executorconformance": 10, "internal/executor/fixture": 10, "internal/executor/native": 13, "internal/executor/native/internal/domain": 10, "internal/executor/native/internal/transport": 11, "internal/executor/native/internal/store": 11, "internal/executor/native/internal/app": 12, "internal/executor/remote": 13, "internal/executor/remote/internal/domain": 10, "internal/executor/remote/internal/transport": 11, "internal/executor/remote/internal/app": 12, "internal/runartifact": 10, "internal/runartifact/internal/domain": 2, "internal/runartifact/internal/transport": 3, "internal/runartifact/internal/store": 8, "internal/runartifact/internal/app": 9,
	"internal/replay": 16, "internal/replay/internal/domain": 5, "internal/replay/internal/transport": 8, "internal/replay/internal/store": 14, "internal/replay/internal/app": 15, "internal/runtime": 17, "cmd/agentic-stream": 18,
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
