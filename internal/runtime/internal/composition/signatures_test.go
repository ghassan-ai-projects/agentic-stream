package composition

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/playbook"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestRuleApprovalSignaturesAreCheckedByPolicy(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	approval := playbook.SignedApproval{TenantID: "t", Approver: "operator", Relay: "relay", RiskClass: "R1", Message: []byte("m"), Signature: []byte("s")}
	err := db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if err := (policySignatures{}).CheckSigner(t.Context(), tx, approval); !errors.Is(err, policy.ErrApprovalUnauthorized) {
			t.Fatalf("an unknown signer = %v, want policy's refusal", err)
		}
		if _, err := (policySignatures{}).Verify(t.Context(), tx, approval); !errors.Is(err, policy.ErrApprovalUnauthorized) {
			t.Fatalf("an unknown signer's signature = %v, want policy's refusal", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
