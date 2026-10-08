package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

// Ownership fences a governance change: Check must confirm that Epoch owns
// the runtime lease inside the change's transaction.
type Ownership struct {
	Check store.Fence
	Epoch string
}

// ApplyPrincipals makes the tenant's approval governance match the document,
// inside the caller's transaction, after the runtime owner fence.
func ApplyPrincipals(ctx context.Context, tx *store.Tx, ownership Ownership, document domain.PrincipalDocument, now time.Time) (domain.PrincipalSummary, error) {
	if ownership.Check == nil || ownership.Epoch == "" {
		return domain.PrincipalSummary{}, fmt.Errorf("principal changes need the runtime owner fence")
	}
	if err := tx.Assert(ctx, ownership.Check, ownership.Epoch); err != nil {
		return domain.PrincipalSummary{}, fmt.Errorf("principal changes need runtime ownership: %w", err)
	}
	if err := document.Validate(); err != nil {
		return domain.PrincipalSummary{}, err //nolint:wrapcheck // The domain rule's message is the operator-facing text.
	}
	return tx.ReplaceGovernance(ctx, document, domain.FormatTime(now))
}

// GovernanceSummary counts the tenant's stored approval governance.
func GovernanceSummary(ctx context.Context, tx *store.Tx, tenant string) (domain.PrincipalSummary, error) {
	return tx.GovernanceSummary(ctx, tenant)
}
