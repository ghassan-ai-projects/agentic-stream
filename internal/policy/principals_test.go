package policy_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
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
	db, err := storagetest.Open(context.Background(), filepath.Join(t.TempDir(), "governance.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
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
	refused := policy.Ownership{Check: func(context.Context, *sql.Tx, string) error { return errors.New("runtime running") }, Epoch: "operator"}
	if _, err := apply(t, db, refused, full); err == nil {
		t.Fatal("a change without runtime ownership was applied")
	}
}
