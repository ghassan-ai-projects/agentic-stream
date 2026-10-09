package architecture

import (
	"go/ast"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestPackageLayering enforces quality-bar rule Q5: production imports between
// this module's packages follow the reviewed dependency graph.
func TestPackageLayering(t *testing.T) {
	t.Parallel()

	graph := loadRepository(t).graph
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
			case imported == kernelPackage:
				continue
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

	graph := loadRepository(t).graph
	for _, pkg := range slices.Sorted(maps.Keys(allowedImports)) {
		for _, allowed := range allowedImports[pkg] {
			if !slices.Contains(graph[pkg], allowed) {
				t.Errorf("allowedImports approves %s -> %s, which no production file uses; remove the stale edge", pkg, allowed)
			}
		}
	}
}

func TestImportsOnlyPointToLowerArchitectureLayers(t *testing.T) {
	t.Parallel()

	graph := loadRepository(t).graph
	for _, pkg := range slices.Sorted(maps.Keys(graph)) {
		layer, ok := packageLayers[pkg]
		if !ok {
			t.Errorf("%s has no reviewed architecture layer", pkg)
			continue
		}
		for _, dependency := range graph[pkg] {
			lower, ok := packageLayers[dependency]
			if !ok {
				t.Errorf("%s depends on unclassified %s", pkg, dependency)
				continue
			}
			if dependency != kernelPackage && lower >= layer {
				t.Errorf("%s (layer %d) depends upward/sideways on %s (layer %d)", pkg, layer, dependency, lower)
			}
		}
	}
	for _, pkg := range slices.Sorted(maps.Keys(packageLayers)) {
		if _, ok := graph[pkg]; !ok {
			t.Errorf("stale architecture layer: %s", pkg)
		}
	}
}

var reasoningPackages = []string{
	"internal/cognition", "internal/decisions", "internal/engine", "internal/engine/internal/app", "internal/engine/internal/domain", "internal/engine/internal/store",
	"internal/episodes", "internal/evidence",
	"internal/executor/native", "internal/executor/native/internal/app", "internal/executor/native/internal/domain", "internal/executor/native/internal/store", "internal/executor/native/internal/transport",
	"internal/executor/remote", "internal/executor/remote/internal/app", "internal/executor/remote/internal/domain", "internal/executor/remote/internal/transport",
	"internal/testsupport/executorconformance", "internal/worker",
}

var replayPackages = []string{"internal/replay", "internal/replay/internal/app", "internal/replay/internal/domain", "internal/replay/internal/store", "internal/replay/internal/transport"}

var effectImplementationPackages = []string{
	"internal/actions", "internal/actions/internal/app", "internal/actions/internal/store",
	"internal/device", "internal/device/internal/app", "internal/device/internal/transport",
	"internal/watch",
	"internal/runtime", "internal/runtime/internal/app", "internal/runtime/internal/composition", "internal/runtime/internal/store", "internal/runtime/internal/transport",
	"cmd/agentic-stream",
}

var policyPackages = []string{"internal/policy", "internal/policy/internal/app", "internal/policy/internal/store", "internal/policy/internal/domain"}

var deviceAdapterPackages = []string{"internal/device", "internal/device/internal/app"}

var upstreamServicePackages = []string{"internal/actions", "internal/policy", "internal/episodes", "internal/runtime"}

func TestReasoningAndReplayCannotReachEffectImplementations(t *testing.T) {
	t.Parallel()

	graph := loadRepository(t).graph
	for _, source := range slices.Concat(reasoningPackages, replayPackages) {
		for _, target := range effectImplementationPackages {
			assertUnreachable(t, graph, "effect implementation reachable", source, target)
		}
	}
	for _, source := range reasoningPackages {
		for _, target := range policyPackages {
			assertUnreachable(t, graph, "reasoning reaches policy", source, target)
		}
	}
	for _, adapter := range deviceAdapterPackages {
		for _, target := range upstreamServicePackages {
			assertUnreachable(t, graph, "device adapter reaches upstream service", adapter, target)
		}
	}
}

func assertUnreachable(t *testing.T, graph map[string][]string, message, source, target string) {
	t.Helper()

	if route := dependencyPath(graph, source, target); len(route) > 0 {
		t.Errorf("%s: %s", message, strings.Join(route, " -> "))
	}
}

func dependencyPath(graph map[string][]string, source, target string) []string {
	queue := [][]string{{source}}
	visited := map[string]bool{source: true}
	for len(queue) > 0 {
		route := queue[0]
		queue = queue[1:]
		node := route[len(route)-1]
		if node == target {
			return route
		}
		for _, next := range graph[node] {
			if !visited[next] {
				visited[next] = true
				queue = append(queue, append(slices.Clone(route), next))
			}
		}
	}
	return nil
}

func TestDependencyReachabilityIncludesIndirectAndCyclicPaths(t *testing.T) {
	t.Parallel()

	graph := map[string][]string{"reasoning": {"ledger"}, "ledger": {"reasoning", "adapter"}}
	if route := dependencyPath(graph, "reasoning", "adapter"); !slices.Equal(route, []string{"reasoning", "ledger", "adapter"}) {
		t.Fatalf("indirect path = %v, want [reasoning ledger adapter]", route)
	}
	if route := dependencyPath(graph, "reasoning", "unreachable"); len(route) != 0 {
		t.Fatalf("cycle produced false path = %v, want none", route)
	}
}

func TestContractPackagesExcludePersistenceAndTransport(t *testing.T) {
	t.Parallel()

	repo := loadRepository(t)
	for _, pkg := range []string{"internal/actionport", "internal/contractsv1"} {
		for _, file := range repo.filesIn(t, pkg) {
			for _, name := range file.imports {
				if name == "database/sql" || name == "net" || strings.HasPrefix(name, "net/") || strings.HasSuffix(name, "/internal/storage") {
					t.Errorf("contract package %s imports implementation %s", pkg, name)
				}
			}
		}
	}
}

func TestFinalAuthorizationIsConstructedOnlyByLowerControlCapability(t *testing.T) {
	t.Parallel()

	repo := loadRepository(t)
	for _, file := range repo.production {
		if file.dir == "internal/control" {
			continue
		}
		aliases := importAliases(file, repo.module+"/internal/actionport", "actionport")
		ast.Inspect(file.syntax, func(node ast.Node) bool {
			if constructsAuthorization(node, aliases) {
				t.Errorf("%s constructs an upstream authorization callback; use control.NewDispatchAuthorization", file.rel)
			}
			return true
		})
	}
}

// importAliases names the identifiers under which the file imports target.
func importAliases(file goFile, target, defaultName string) map[string]bool {
	aliases := map[string]bool{}
	for _, spec := range file.syntax.Imports {
		name, _ := strconv.Unquote(spec.Path.Value)
		if name != target {
			continue
		}
		alias := defaultName
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		aliases[alias] = true
	}
	return aliases
}

func constructsAuthorization(node ast.Node, aliases map[string]bool) bool {
	literal, ok := node.(*ast.CompositeLit)
	if !ok {
		return false
	}
	typ, ok := literal.Type.(*ast.SelectorExpr)
	if !ok || typ.Sel.Name != "Authorization" {
		return false
	}
	alias, ok := typ.X.(*ast.Ident)
	return ok && aliases[alias.Name]
}

// executorTransportImports are the worker protocol and transport packages
// that only a concrete executor under internal/executor may import.
var executorTransportImports = []string{"google.golang.org/grpc", "google.golang.org/protobuf", "/internal/worker", "/proto/"}

// TestEpisodeLifecycleImportsNoExecutorTransport enforces architecture-bar
// rule A8: the episode lifecycle depends on the Executor port only, so it
// classifies failures without knowing how a concrete executor communicates.
func TestEpisodeLifecycleImportsNoExecutorTransport(t *testing.T) {
	t.Parallel()

	for _, file := range loadRepository(t).filesUnder("internal/episodes") {
		for _, imported := range file.imports {
			if isExecutorTransport(imported) {
				t.Errorf("%s imports executor transport %s; move it to an executor under internal/executor", file.rel, imported)
			}
		}
	}
}

func isExecutorTransport(imported string) bool {
	return slices.ContainsFunc(executorTransportImports, func(transport string) bool {
		return strings.Contains(imported, transport)
	})
}
