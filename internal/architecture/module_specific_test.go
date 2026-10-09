package architecture

import (
	"go/ast"
	"path"
	"slices"
	"strings"
	"testing"
)

// TestApprovalLedgerDoesNotImportTransport keeps the lifecycle owner below the notification outbox.
func TestApprovalLedgerDoesNotImportTransport(t *testing.T) {
	t.Parallel()

	repo := loadRepository(t)
	for _, file := range repo.filesUnder("internal/approvalledger") {
		if !slices.Contains([]string{"internal/approvalledger", "internal/approvalledger/internal/app", "internal/approvalledger/internal/domain", "internal/approvalledger/internal/store"}, file.dir) {
			continue
		}
		if slices.Contains(file.imports, repo.module+"/internal/notify") {
			t.Errorf("%s imports the notification outbox; the caller supplies the publisher", file.rel)
		}
	}
}

// TestDecisionCatalogKeepsAuthorityPrivate prevents public mutable compiled rules.
func TestDecisionCatalogKeepsAuthorityPrivate(t *testing.T) {
	t.Parallel()

	for _, file := range loadRepository(t).filesIn(t, "internal/decisions/internal/domain") {
		ast.Inspect(file.syntax, func(node ast.Node) bool {
			declaration, ok := node.(*ast.TypeSpec)
			if !ok || declaration.Name.Name != "IntentCatalog" {
				return true
			}
			record, ok := declaration.Type.(*ast.StructType)
			if declaration.Assign.IsValid() || !ok {
				t.Errorf("%s: catalog must encapsulate compiled authority", file.rel)
				return true
			}
			for _, field := range record.Fields.List {
				if len(field.Names) == 0 {
					t.Errorf("%s: catalog must not embed authority", file.rel)
				}
				for _, name := range field.Names {
					if name.IsExported() {
						t.Errorf("%s: catalog authority field %s must be private", file.rel, name)
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

	repo := loadRepository(t)
	for _, layer := range []string{"app", "domain", "store"} {
		for _, file := range repo.filesIn(t, "internal/evidence/internal/"+layer) {
			for _, dependency := range file.imports {
				if strings.HasPrefix(dependency, "google.golang.org/") || strings.HasPrefix(dependency, repo.module+"/proto/") {
					t.Errorf("%s: protocol import %s belongs in an adapter", file.rel, dependency)
				}
				if slices.Contains([]string{"encoding/json", "encoding/base64", "crypto/hmac", "crypto/rand"}, dependency) {
					t.Errorf("%s: codec import %s belongs in wire", file.rel, dependency)
				}
			}
		}
	}
}

// ledgerPureHelpers are the episode ledger functions that read no database and
// may be called from outside the ledger.
var ledgerPureHelpers = []string{"IsTerminalAttempt", "RejectionReason"}

// TestEpisodeApplicationCallsLedgerOnlyThroughPureHelpers keeps the episode
// lifecycle from calling the transactional episode ledger directly, even
// through an import alias.
func TestEpisodeApplicationCallsLedgerOnlyThroughPureHelpers(t *testing.T) {
	t.Parallel()

	repo := loadRepository(t)
	for _, file := range repo.filesIn(t, "internal/episodes/internal/app") {
		owners := importOwners(file, repo.module)
		for _, call := range calledSelectors(file.syntax) {
			receiver, ok := call.X.(*ast.Ident)
			if ok && owners[receiver.Name] == "internal/episodeledger" && !slices.Contains(ledgerPureHelpers, call.Sel.Name) {
				t.Errorf("%s: app calls transactional owner %s.%s directly", file.rel, owners[receiver.Name], call.Sel.Name)
			}
		}
	}
}

// importOwners maps each import name of the file, alias included, to the
// import path relative to the module.
func importOwners(file goFile, module string) map[string]string {
	owners := map[string]string{}
	for i, spec := range file.syntax.Imports {
		name := path.Base(file.imports[i])
		if spec.Name != nil {
			name = spec.Name.Name
		}
		owners[name] = strings.TrimPrefix(file.imports[i], module+"/")
	}
	return owners
}
