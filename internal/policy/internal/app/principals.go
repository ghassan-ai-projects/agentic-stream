package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

type Ownership struct {
	Check store.OwnerCheck
	Epoch string
}

func ApplyPrincipals(ctx context.Context, tx *store.Tx, ownership Ownership, document domain.PrincipalDocument, now time.Time) (domain.PrincipalSummary, error) {
	if ownership.Check == nil || ownership.Epoch == "" {
		return domain.PrincipalSummary{}, fmt.Errorf("principal changes need the runtime owner fence")
	}
	if err := tx.Assert(ctx, ownership.Check, ownership.Epoch); err != nil {
		return domain.PrincipalSummary{}, fmt.Errorf("principal changes need runtime ownership: %w", err)
	}
	if err := document.Validate(); err != nil {
		return domain.PrincipalSummary{}, fmt.Errorf("validate principal document: %w", err)
	}
	return tx.ReplaceGovernance(ctx, document, now)
}

func GovernanceSummary(ctx context.Context, tx *store.Tx, tenant string) (domain.PrincipalSummary, error) {
	return tx.GovernanceSummary(ctx, tenant)
}
