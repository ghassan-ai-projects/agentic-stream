package policy

import (
	"context"
	"database/sql"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

// PrincipalDocument declares a tenant's approval relays, approvers, roles and
// the entities and risks each role may approve.
type PrincipalDocument = domain.PrincipalDocument

// PrincipalSummary counts the tenant's governance after an apply.
type PrincipalSummary = domain.PrincipalSummary

// Ownership fences a governance change with the runtime owner lease: Check is
// the owner's assertion and Epoch the lease holder.
type Ownership = app.Ownership

// ParsePrincipals decodes and validates a YAML principal document.
func ParsePrincipals(data []byte) (PrincipalDocument, error) {
	return domain.ParsePrincipalDocument(data) //nolint:wrapcheck // The domain rule's message is the operator-facing text.
}

// ApplyPrincipals makes the tenant's approval governance match the document
// in the caller's transaction, after the ownership fence. Omitted principals
// are disabled, never deleted.
func ApplyPrincipals(ctx context.Context, tx *sql.Tx, ownership Ownership, document PrincipalDocument, now time.Time) (PrincipalSummary, error) {
	return app.ApplyPrincipals(ctx, store.Join(tx), ownership, document, now)
}

// GovernanceSummary counts the tenant's stored approval governance.
func GovernanceSummary(ctx context.Context, tx *sql.Tx, tenant string) (PrincipalSummary, error) {
	return app.GovernanceSummary(ctx, store.Join(tx), tenant)
}
