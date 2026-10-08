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
	assertStoreEncapsulatesInfrastructure(t, "internal/evidence/internal/store")
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
	assertApplicationAvoidsCalls(t, "internal/evidence/internal/app", []string{"Exec", "ExecContext", "QueryContext", "QueryRow", "QueryRowContext", "Begin", "BeginTx", "Commit", "Rollback"})
}
