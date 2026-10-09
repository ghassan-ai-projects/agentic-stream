package policy_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

const approverKey = "O2onvM62pC1io6jQKm8Nc2UyFXcd4kOmOsBIoYtZ2ik="

var passFence = policy.Ownership{Check: func(context.Context, *sql.Tx, string) error { return nil }, Epoch: "test"}

func governanceDocument(t *testing.T, tenant string, principals string) policy.PrincipalDocument {
	t.Helper()
	document, err := policy.ParsePrincipals([]byte("tenant: " + tenant + "\nprincipals:\n" + principals))
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func apply(t *testing.T, db *storage.DB, fence policy.Ownership, document policy.PrincipalDocument) (policy.PrincipalSummary, error) {
	t.Helper()
	var summary policy.PrincipalSummary
	err := db.WithTx(t.Context(), func(tx *sql.Tx) (err error) {
		summary, err = policy.ApplyPrincipals(t.Context(), tx, fence, document, time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC))
		return err
	})
	return summary, err
}

func TestApplyPrincipalsMakesGovernanceMatchTheDocument(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	full := governanceDocument(t, "default", "  - id: relay\n  - id: alice\n    public_key: "+approverKey+"\nroles:\n  - id: r\n    name: thermal\n    members: [alice]\n    authorities:\n      - entity: zone-01\n        risks: [R1, R2]\n")
	first, err := apply(t, db, passFence, full)
	if err != nil || first.Active != 2 || first.Roles != 1 || first.Memberships != 1 || first.Authorities != 2 {
		t.Fatalf("first apply = %+v, %v", first, err)
	}
	again, err := apply(t, db, passFence, full)
	if err != nil || again != first {
		t.Fatalf("re-apply = %+v, %v; want %+v", again, err, first)
	}
	reduced, err := apply(t, db, passFence, governanceDocument(t, "default", "  - id: relay\n"))
	if err != nil || reduced.Active != 1 || reduced.Disabled != 1 || reduced.Memberships != 0 || reduced.Authorities != 0 {
		t.Fatalf("reduced apply = %+v, %v; omitted principals must be disabled, not deleted", reduced, err)
	}
	if _, err := apply(t, db, passFence, governanceDocument(t, "other", "  - id: relay\n")); err == nil {
		t.Fatal("a principal moved to another tenant")
	}
}

func TestApplyPrincipalsRequiresTheRuntimeOwnerFence(t *testing.T) {
	t.Parallel()
	document := governanceDocument(t, "default", "  - id: relay\n")
	notRunning := errors.New("runtime running")
	tests := []struct {
		name  string
		fence policy.Ownership
		want  string
	}{
		{"no check", policy.Ownership{Epoch: "operator"}, "need the runtime owner fence"},
		{"no epoch", policy.Ownership{Check: passFence.Check}, "need the runtime owner fence"},
		{"the owner lease is held elsewhere", policy.Ownership{Check: func(context.Context, *sql.Tx, string) error { return notRunning }, Epoch: "operator"}, "need runtime ownership"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			db := storagetest.OpenTemp(t)
			if _, err := apply(t, db, test.fence, document); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("apply = %v, want a refusal mentioning %q", err, test.want)
			}
			if summary := governanceSummary(t, db, "default"); summary != (policy.PrincipalSummary{Tenant: "default"}) {
				t.Fatalf("a refused apply left governance %+v", summary)
			}
		})
	}
}

func TestGovernanceSummaryCountsOnlyTheTenantsStoredGovernance(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	document := governanceDocument(t, "default", "  - id: relay\n  - id: alice\n    public_key: "+approverKey+"\nroles:\n  - id: r\n    name: thermal\n    members: [alice]\n    authorities:\n      - entity: zone-01\n        risks: [R2]\n")
	if _, err := apply(t, db, passFence, document); err != nil {
		t.Fatal(err)
	}

	want := policy.PrincipalSummary{Tenant: "default", Active: 2, Roles: 1, Memberships: 1, Authorities: 1}
	if got := governanceSummary(t, db, "default"); got != want {
		t.Fatalf("summary of default = %+v, want %+v", got, want)
	}
	if got := governanceSummary(t, db, "other"); got != (policy.PrincipalSummary{Tenant: "other"}) {
		t.Fatalf("summary of another tenant = %+v, want empty", got)
	}
}

func TestParsePrincipalsNamesTheDocumentWhenItIsInvalid(t *testing.T) {
	t.Parallel()
	if _, err := policy.ParsePrincipals([]byte("tenant: \"\"\n")); err == nil || !strings.Contains(err.Error(), "parse principal document") {
		t.Fatalf("ParsePrincipals = %v, want a wrapped refusal", err)
	}
}

func governanceSummary(t *testing.T, db *storage.DB, tenant string) policy.PrincipalSummary {
	t.Helper()
	var summary policy.PrincipalSummary
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) (err error) {
		summary, err = policy.GovernanceSummary(t.Context(), tx, tenant)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return summary
}
