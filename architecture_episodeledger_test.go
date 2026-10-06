package agenticstream

import (
	"go/ast"
	"path"
	"slices"
	"testing"
)

// episodeLedgerOperations are the facade functions; each delegates to the app
// layer (the pure helpers delegate to the domain).
var episodeLedgerOperations = []string{
	"Admit", "StartAttempt", "StartAttemptOwned", "TransitionAttempt", "ValidateWorkerIdentity", "RecordRejection", "RecoverUnfinishedAttempts",
	"Rebind", "BindRequest", "AbandonRebind", "Abandon", "Conclude", "RetainForRetry", "SupersedeEpoch", "SupersedeCoalesced",
	"UpsertSchedulerItem", "MarkSchedulerItemAdmitted", "CoalesceSchedulerItems", "CoalesceCostRejectedItem", "CoalesceSkippedItem", "NextPendingSchedulerItem",
}

// TestEpisodeLedgerFacadeOnlyDelegates confines ledger rules and SQL to private layers.
func TestEpisodeLedgerFacadeOnlyDelegates(t *testing.T) {
	t.Parallel()
	for _, file := range productionGoFiles(t, repoRoot(t)) {
		if path.Dir(file.rel) != "internal/episodeledger" {
			continue
		}
		for _, declaration := range parseGoFile(t, file).Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			target := "app"
			if slices.Contains([]string{"IsIdentityReason", "CanTransitionAttempt", "IsTerminalAttempt"}, function.Name.Name) {
				target = "domain"
			} else if !slices.Contains(episodeLedgerOperations, function.Name.Name) {
				t.Errorf("%s: %s is not an episode ledger operation", file.rel, function.Name)
				continue
			}
			if !facadeDelegation(function, target) {
				t.Errorf("%s: %s must only delegate through the episode ledger facade", file.rel, function.Name)
			}
		}
	}
}

// TestEpisodeLedgerStoreKeepsTransactionsOpaque protects the caller's transaction.
func TestEpisodeLedgerStoreKeepsTransactionsOpaque(t *testing.T) {
	t.Parallel()
	assertStoreEncapsulatesInfrastructure(t, "internal/episodeledger/internal/store")
}

// TestEpisodeLedgerApplicationUsesTransactionalPorts keeps SQL behind named store operations.
func TestEpisodeLedgerApplicationUsesTransactionalPorts(t *testing.T) {
	t.Parallel()
	assertApplicationUsesOpaquePorts(t, "internal/episodeledger/internal/app")
}
