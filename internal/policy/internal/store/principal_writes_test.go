package store

import (
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func replaceGovernance(t *testing.T, db *storage.DB, document domain.PrincipalDocument) (domain.PrincipalSummary, error) {
	t.Helper()
	var summary domain.PrincipalSummary
	err := db.WithTx(t.Context(), func(tx *sql.Tx) (err error) {
		summary, err = Join(tx).ReplaceGovernance(t.Context(), document, fixtureNow)
		return err
	})
	return summary, err
}

func approverEntry(t *testing.T, id string) domain.PrincipalEntry {
	t.Helper()
	public, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return domain.PrincipalEntry{ID: id, PublicKey: base64.StdEncoding.EncodeToString(public)}
}

func TestReplacingGovernanceDisablesOmittedPrincipalsAndReplacesAllGrants(t *testing.T) {
	t.Parallel()
	db, _ := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	document := domain.PrincipalDocument{
		Tenant:     "tenant",
		Principals: []domain.PrincipalEntry{{ID: "relay-1"}, approverEntry(t, "approver-2")},
		Roles:      []domain.RoleEntry{{ID: "role-ops", Name: "ops", Members: []string{"approver-2"}, Authorities: []domain.AuthorityEntry{{Entity: "motor-1", Risks: []string{"R1", "R2"}}}}},
	}

	summary, err := replaceGovernance(t, db, document)
	want := domain.PrincipalSummary{Tenant: "tenant", Active: 2, Disabled: 1, Roles: 1, Memberships: 1, Authorities: 2}
	if err != nil || summary != want {
		t.Fatalf("summary = %+v, %v; want %+v", summary, err, want)
	}
	if status := scalar[string](t, db, "SELECT status FROM principals WHERE principal_id = 'operator-1'"); status != "disabled" {
		t.Fatalf("the omitted approver is %q, want disabled and kept for the audit trail", status)
	}
	if old := scalar[int](t, db, "SELECT COUNT(*) FROM approval_authorities WHERE role_id = 'role-approver'"); old != 0 {
		t.Fatalf("%d grants of the replaced role survived", old)
	}
}

func TestReplacingGovernanceNeverTakesOverAnotherTenantsPrincipal(t *testing.T) {
	t.Parallel()
	db, _ := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	if _, err := db.ExecContext(t.Context(), "INSERT INTO principals (principal_id, tenant_id, status, created_at) VALUES ('shared', 'other', 'active', '2026-08-12T00:00:00Z')"); err != nil {
		t.Fatal(err)
	}

	_, err := replaceGovernance(t, db, domain.PrincipalDocument{Tenant: "tenant", Principals: []domain.PrincipalEntry{{ID: "shared"}}})
	if err == nil || !strings.Contains(err.Error(), "belongs to another tenant") {
		t.Fatalf("replace = %v, want a refusal naming the other tenant", err)
	}
	if tenant := scalar[string](t, db, "SELECT tenant_id FROM principals WHERE principal_id = 'shared'"); tenant != "other" {
		t.Fatalf("principal moved to tenant %q", tenant)
	}
	if status := scalar[string](t, db, "SELECT status FROM principals WHERE principal_id = 'operator-1'"); status != "active" {
		t.Fatalf("a refused replacement left operator-1 %q, want the whole change rolled back", status)
	}
}

func TestAGrantToAnUndeclaredPrincipalIsRefusedAndRolledBack(t *testing.T) {
	t.Parallel()
	db, _ := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	document := domain.PrincipalDocument{
		Tenant:     "tenant",
		Principals: []domain.PrincipalEntry{{ID: "relay-1"}},
		Roles:      []domain.RoleEntry{{ID: "role-ops", Name: "ops", Members: []string{"ghost"}}},
	}

	if _, err := replaceGovernance(t, db, document); err == nil || !strings.Contains(err.Error(), "grant role role-ops to ghost") {
		t.Fatalf("replace = %v, want a refusal naming the grant", err)
	}
	if active := scalar[int](t, db, "SELECT COUNT(*) FROM principals WHERE status = 'active'"); active != 2 {
		t.Fatalf("active principals = %d after a refused replacement, want the original 2", active)
	}
}

func TestGovernanceSummaryCountsOneTenantAtATime(t *testing.T) {
	t.Parallel()
	db, _ := openPolicyFixture(t, "R1", 1, 1, fixtureNow.Add(time.Hour))
	var tenant, other domain.PrincipalSummary
	inJoined(t, db, func(tx *Tx) (err error) {
		if tenant, err = tx.GovernanceSummary(t.Context(), "tenant"); err != nil {
			return err
		}
		other, err = tx.GovernanceSummary(t.Context(), "other")
		return err
	})
	if want := (domain.PrincipalSummary{Tenant: "tenant", Active: 2, Roles: 1, Memberships: 1, Authorities: 1}); tenant != want {
		t.Fatalf("tenant summary = %+v, want %+v", tenant, want)
	}
	if other != (domain.PrincipalSummary{Tenant: "other"}) {
		t.Fatalf("other tenant summary = %+v, want empty", other)
	}
}
