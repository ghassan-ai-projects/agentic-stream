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
	"internal/runtime/internal/domain": 1, "internal/runtime/internal/transport": 9, "internal/runtime/internal/store": 9, "internal/runtime/internal/app": 11, "internal/runtime/internal/composition": 12,
	"internal/actionport": 0, "internal/canonicaljson": 0, "internal/clock": 0, "internal/costcontrol": 0,
	"internal/duration": 0, "internal/eventschema": 0, "internal/ids": 0, "internal/interlock": 0, "internal/telemetry": 0,
	"migrations": 0, "proto/agenticstream/runtime/v1": 0,
	"internal/authority/internal/domain": 1, "internal/contractsv1": 1, "internal/storage": 1,
	"internal/policy/internal/domain": 2, "internal/decisions": 3, "internal/decisions/internal/domain": 2, "internal/device/internal/domain": 2, "internal/device/internal/wire": 3, "internal/episodeledger": 2, "internal/eventlog": 4, "internal/evidence": 6, "internal/evidence/internal/domain": 2, "internal/evidence/internal/wire": 3, "internal/evidence/internal/store": 3, "internal/evidence/internal/app": 4, "internal/evidence/internal/transport": 5,
	"internal/authority/internal/store": 2, "internal/qualification": 2, "internal/scheduleledger": 2, "internal/spec": 2, "internal/worker": 2,
	"internal/authority/internal/app": 3, "internal/control": 3, "internal/device/internal/transport": 4, "internal/eventlog/internal/domain": 1, "internal/eventlog/internal/store": 2, "internal/eventlog/internal/app": 3, "internal/ingress": 6, "internal/ingress/internal/domain": 2, "internal/ingress/internal/store": 3, "internal/ingress/internal/transport": 3, "internal/ingress/internal/app": 5, "internal/notify": 4, "internal/notify/internal/domain": 2, "internal/notify/internal/store": 2, "internal/notify/internal/app": 3, "internal/operators": 3,
	"internal/api": 5, "internal/approvalledger": 5, "internal/watch": 5, "internal/watch/internal/domain": 2, "internal/watch/internal/store": 3, "internal/watch/internal/app": 4, "internal/authority": 4, "internal/situations": 4,
	"internal/actions": 7, "internal/actions/internal/domain": 2, "internal/actions/internal/store": 5, "internal/actions/internal/app": 6, "internal/cognition": 8, "internal/cognition/internal/domain": 5, "internal/cognition/internal/store": 6, "internal/cognition/internal/app": 7, "internal/device/internal/app": 5, "internal/episodes": 7, "internal/episodes/internal/domain": 4, "internal/episodes/internal/store": 5, "internal/episodes/internal/app": 6, "internal/policy/internal/store": 6, "internal/policy/internal/app": 7, "internal/policy": 8, "internal/soak": 5,
	"internal/admission": 9, "internal/device": 6, "internal/engine": 10, "internal/engine/internal/domain": 5, "internal/engine/internal/store": 6, "internal/engine/internal/app": 9, "internal/executor/conformance": 8, "internal/executor/fixture": 8, "internal/executor/native": 8, "internal/executor/remote": 8, "internal/runartifact": 9,
	"internal/replay": 13, "internal/replay/internal/domain": 4, "internal/replay/internal/transport": 7, "internal/replay/internal/store": 11, "internal/replay/internal/app": 12, "internal/runtime": 13, "cmd/agentic-stream": 14,
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
	reasoning := []string{"internal/cognition", "internal/decisions", "internal/engine", "internal/engine/internal/app", "internal/engine/internal/domain", "internal/engine/internal/store", "internal/episodes", "internal/evidence", "internal/executor/native", "internal/executor/remote", "internal/executor/conformance", "internal/worker"}
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
