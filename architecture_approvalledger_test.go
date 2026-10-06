package agenticstream

import (
	"go/ast"
	"path"
	"slices"
	"testing"
)

// approvalLedgerOperations are the facade functions; each delegates to the app layer.
var approvalLedgerOperations = []string{"Request", "ExpireIntent", "Expire", "Resolve", "BindAssertion", "Withdraw", "WithdrawSuperseded"}

// TestApprovalLedgerFacadeOnlyDelegates confines approval rules and SQL to private layers.
func TestApprovalLedgerFacadeOnlyDelegates(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/approvalledger" {
			continue
		}
		for _, declaration := range parseGoFile(t, file).Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if !slices.Contains(approvalLedgerOperations, function.Name.Name) || !facadeDelegation(function, "app") {
				t.Errorf("%s: %s must only delegate through the approval ledger facade", file.rel, function.Name)
			}
		}
	}
}

// TestApprovalLedgerStoreKeepsTransactionsOpaque protects the caller's transaction.
func TestApprovalLedgerStoreKeepsTransactionsOpaque(t *testing.T) {
	t.Parallel()
	assertStoreEncapsulatesInfrastructure(t, "internal/approvalledger/internal/store")
}

// TestApprovalLedgerApplicationUsesTransactionalPorts keeps SQL behind named store operations.
func TestApprovalLedgerApplicationUsesTransactionalPorts(t *testing.T) {
	t.Parallel()
	assertApplicationUsesOpaquePorts(t, "internal/approvalledger/internal/app")
}

// TestApprovalLedgerDoesNotImportTransport keeps the lifecycle owner below the notification outbox.
func TestApprovalLedgerDoesNotImportTransport(t *testing.T) {
	t.Parallel()
	module := readModulePath(t, path.Join(repoRoot(t), "go.mod"))
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if !slices.Contains([]string{"internal/approvalledger", "internal/approvalledger/internal/app", "internal/approvalledger/internal/domain", "internal/approvalledger/internal/store"}, path.Dir(file.rel)) {
			continue
		}
		for _, imported := range importPaths(t, file, parseGoFile(t, file)) {
			if imported == module+"/internal/notify" {
				t.Errorf("%s imports the notification outbox; the caller supplies the publisher", file.rel)
			}
		}
	}
}
