package agenticstream

import (
	"go/ast"
	"path"
	"slices"
	"strings"
	"testing"
)

// TestEvidenceFacadeOnlyDelegates keeps rules and codecs behind the public service.
func TestEvidenceFacadeOnlyDelegates(t *testing.T) {
	t.Parallel()
	construction := []string{"New", "applicationConfig", "ledgerConfig", "callConfig", "privateService"}
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/evidence" {
			continue
		}
		for _, declaration := range parseGoFile(t, file).Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			target := evidenceDelegationTarget(function.Name.Name)
			if function.Recv == nil && slices.Contains(construction, function.Name.Name) {
				continue
			}
			if target == "" || !facadeDelegation(function, target) {
				t.Errorf("%s: %s must only delegate through the evidence facade", file.rel, function.Name)
			}
		}
	}
}
func evidenceDelegationTarget(operation string) string {
	switch operation {
	case "Issue", "Verify", "IssueTime", "RuntimeEpoch", "ReclaimExpired", "RecoverTx":
		return "app"
	case "Call", "EventLogQuery":
		return "transport"
	case "NewRuntimeEpoch":
		return "wire"
	default:
		return ""
	}
}

// TestEvidenceStoreKeepsInfrastructurePrivate prevents raw transaction aliases and handles.
func TestEvidenceStoreKeepsInfrastructurePrivate(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/evidence/internal/store" {
			continue
		}
		ast.Inspect(parseGoFile(t, file), func(node ast.Node) bool {
			declaration, ok := node.(*ast.TypeSpec)
			if !ok || !slices.Contains([]string{"Store", "Tx"}, declaration.Name.Name) {
				return true
			}
			record, ok := declaration.Type.(*ast.StructType)
			if declaration.Assign.IsValid() || !ok {
				t.Errorf("%s: %s must encapsulate infrastructure", file.rel, declaration.Name)
				return true
			}
			for _, field := range record.Fields.List {
				if len(field.Names) == 0 {
					t.Errorf("%s: %s must not embed infrastructure", file.rel, declaration.Name)
				}
				for _, name := range field.Names {
					if name.IsExported() {
						t.Errorf("%s: infrastructure field %s must be private", file.rel, name)
					}
				}
			}
			return true
		})
	}
}

// TestEvidenceRulesExcludeProtocolAndCodecs keeps authorization transport neutral.
func TestEvidenceRulesExcludeProtocolAndCodecs(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	module := readModulePath(t, path.Join(root, "go.mod"))
	for _, file := range productionGoFiles(t, root) {
		pkg := path.Dir(file.rel)
		if !slices.Contains([]string{"internal/evidence/internal/app", "internal/evidence/internal/domain", "internal/evidence/internal/store"}, pkg) {
			continue
		}
		for _, dependency := range importPaths(t, file, parseGoFile(t, file)) {
			if strings.HasPrefix(dependency, "google.golang.org/") || strings.HasPrefix(dependency, module+"/proto/") {
				t.Errorf("%s: protocol import %s belongs in an adapter", file.rel, dependency)
			}
			if slices.Contains([]string{"encoding/json", "encoding/base64", "crypto/hmac", "crypto/rand"}, dependency) {
				t.Errorf("%s: codec import %s belongs in wire", file.rel, dependency)
			}
		}
	}
}

// TestEvidenceApplicationUsesOpaquePorts prevents infrastructure calls through aliases.
func TestEvidenceApplicationUsesOpaquePorts(t *testing.T) {
	t.Parallel()
	raw := []string{"Exec", "ExecContext", "QueryContext", "QueryRow", "QueryRowContext", "Begin", "BeginTx", "Commit", "Rollback"}
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/evidence/internal/app" {
			continue
		}
		ast.Inspect(parseGoFile(t, file), func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if ok && slices.Contains(raw, selector.Sel.Name) {
				t.Errorf("%s: app calls raw infrastructure operation %s", file.rel, selector.Sel.Name)
			}
			return true
		})
	}
}
