package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
)

// ApprovalForSigning presents immutable evidence after tenant and principal checks.
func (g *Service) ApprovalForSigning(ctx context.Context, tx *store.Tx, r domain.ApprovalLookup) (domain.ApprovalPresentation, error) {
	if err := g.assertOwner(ctx, tx); err != nil {
		return domain.ApprovalPresentation{}, err
	}
	approval, err := tx.LoadApproval(ctx, r.ID, r.TenantID)
	if err != nil {
		return domain.ApprovalPresentation{}, err
	}
	if approval.Status != "pending" {
		return domain.ApprovalPresentation{}, domain.ErrApprovalResolved
	}
	row, err := tx.LoadIntent(ctx, approval.IntentID)
	if err != nil {
		return domain.ApprovalPresentation{}, err
	}
	return presentApproval(ctx, tx, row, approval, r)
}

func presentApproval(ctx context.Context, tx *store.Tx, row domain.IntentRecord, approval domain.ApprovalRecord, r domain.ApprovalLookup) (domain.ApprovalPresentation, error) {
	resolution := domain.ApprovalResolution{ID: r.ID, TenantID: r.TenantID, Approver: r.Approver, Relay: r.Relay, Approved: r.Approved}
	if err := checkSigningPrincipals(ctx, tx, row, resolution); err != nil {
		return domain.ApprovalPresentation{}, fmt.Errorf("%w: %w", domain.ErrApprovalUnauthorized, err)
	}
	signing, err := approvalSigningBytes(ctx, tx, row, resolution)
	if err != nil {
		return domain.ApprovalPresentation{}, err
	}
	return domain.ApprovalPresentation{ID: r.ID, Status: approval.Status, Request: approval.JSON, SigningBytes: signing}, nil
}

func checkSigningPrincipals(ctx context.Context, tx *store.Tx, row domain.IntentRecord, r domain.ApprovalResolution) error {
	if err := domain.DistinctPrincipals(r); err != nil {
		return err
	}
	entity, _, err := loadApprovalPrincipals(ctx, tx, row, r)
	if err != nil {
		return err
	}
	active, err := tx.ApprovalAuthority(ctx, row, entity, r.Approver)
	return domain.AuthorizedApprover(active, err != nil)
}

func assertionFor(row domain.IntentRecord, r domain.ApprovalResolution, expiry time.Time, nonce string) domain.ApprovalAssertion {
	return domain.ApprovalAssertion{
		ApprovalID: r.ID, IntentID: row.IntentID, DecisionID: row.DecisionID, TenantID: row.TenantID, SituationID: row.SituationID, SituationVersion: row.SituationVersion, RiskClass: row.RiskClass,
		IntentDigest: canonicaljson.EncodeDigest(row.IntentSHA), DecisionDigest: canonicaljson.EncodeDigest(row.DecisionSHA), ExpiresAt: expiry, Nonce: nonce, ApproverID: r.Approver, RelayID: r.Relay, Approved: r.Approved,
	}
}
