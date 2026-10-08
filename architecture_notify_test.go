package agenticstream

import (
	"go/ast"
	"path"
	"slices"
	"strings"
	"testing"
)

// TestNotifyFacadeOnlyDelegates confines notification rules and SQL to private layers.
func TestNotifyFacadeOnlyDelegates(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/notify" {
			continue
		}
		for _, declaration := range parseGoFile(t, file).Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || (function.Recv == nil && function.Name.Name == "New") {
				continue
			}
			domainOperation := function.Name.Name == "SourceForTenant" || strings.HasSuffix(function.Name.Name, "Event") && function.Name.Name != "AppendLifecycleEvent"
			if !slices.Contains([]string{"ReadPage", "Prune", "Prunable", "Append", "AppendLifecycleEvent"}, function.Name.Name) && !domainOperation {
				t.Errorf("%s: %s is not a notify operation", file.rel, function.Name)
				continue
			}
			target := "app"
			if domainOperation {
				target = "domain"
			}
			if !facadeDelegation(function, target) {
				t.Errorf("%s: %s must only delegate through the notify facade", file.rel, function.Name)
			}
		}
	}
}

// TestNotifyStoreKeepsTransactionsOpaque protects the notification store's database handle.
func TestNotifyStoreKeepsTransactionsOpaque(t *testing.T) {
	t.Parallel()
	assertStoreEncapsulatesInfrastructure(t, "internal/notify/internal/store")
}

// TestNotifyApplicationUsesTransactionalPorts keeps SQL behind named store operations.
func TestNotifyApplicationUsesTransactionalPorts(t *testing.T) {
	t.Parallel()
	assertApplicationUsesOpaquePorts(t, "internal/notify/internal/app")
}
