package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

func (g *Service) evaluatePending(ctx context.Context, tx *store.Tx, e evaluation) (domain.Result, error) {
	documents, reason := domain.ParseGovernanceDocuments(e.row)
	if reason != "" {
		return g.finish(ctx, tx, e, domain.Outcome{Status: "denied", Reason: reason})
	}
	e.documents = documents
	reason, err := validatePendingIdentity(ctx, tx, e)
	if err != nil {
		return e.result, err
	}
	if reason != "" {
		return g.finish(ctx, tx, e, domain.Outcome{Status: "denied", Reason: reason})
	}
	return g.evaluateFreshPending(ctx, tx, e)
}
func validatePendingIdentity(ctx context.Context, tx *store.Tx, e evaluation) (string, error) {
	if reason, err := compensationFailure(ctx, tx, e); reason != "" || err != nil {
		return reason, err
	}
	if !domain.MatchesIntentIdentity(e.row, e.documents.Intent) {
		return "identity_mismatch", nil
	}
	return "", nil
}
func compensationFailure(ctx context.Context, tx *store.Tx, e evaluation) (string, error) {
	if e.documents.Intent.Compensates == "" {
		return "", nil
	}
	tenant, found, err := tx.CompensationTenant(ctx, e.documents.Intent.Compensates)
	if err != nil {
		return "", err
	}
	return domain.CompensationFailure(e.row.TenantID, tenant, found), nil
}
func (g *Service) evaluateFreshPending(ctx context.Context, tx *store.Tx, e evaluation) (domain.Result, error) {
	status, reason, expires := domain.FreshnessFailure(e.row, e.now)
	if status == "stale" {
		return g.markStale(ctx, tx, e)
	}
	if reason != "" {
		return g.finish(ctx, tx, e, domain.Outcome{Status: status, Reason: reason})
	}
	e.expiresAt = expires
	return g.routeIntent(ctx, tx, e)
}
