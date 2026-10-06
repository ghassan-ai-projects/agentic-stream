package app

import (
	"context"
	"fmt"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/store"
)

// addBoundDefinitions adds the canonical spec and policy and requires the
// manifest to bind both.
func addBoundDefinitions(ctx context.Context, snapshot *store.Snapshot, manifest domain.Manifest, policy domain.PolicyDocuments, files map[string][]byte) error {
	specData, err := canonicalSpec(ctx, snapshot, manifest.TenantID)
	if err != nil {
		return err
	}
	policyData, err := canonicalPolicy(ctx, snapshot, manifest.TenantID, policy)
	if err != nil {
		return err
	}
	files[domain.FileSpec], files[domain.FilePolicy] = specData, policyData
	if err := policy.VerifyBindings(manifest, specData, policyData); err != nil {
		return fmt.Errorf("verify manifest bindings: %w", err)
	}
	return nil
}

func canonicalSpec(ctx context.Context, snapshot *store.Snapshot, tenantID string) ([]byte, error) {
	source, err := snapshot.LatestSpecSource(ctx, tenantID)
	if err != nil {
		return nil, err //nolint:wrapcheck // The store names the failed read.
	}
	if source == nil {
		return domain.CanonicalFile(map[string]any{})
	}
	return domain.CanonicalizeStored(source)
}

func canonicalPolicy(ctx context.Context, snapshot *store.Snapshot, tenantID string, policy domain.PolicyDocuments) ([]byte, error) {
	evaluation, err := snapshot.LatestPolicyEvaluation(ctx, tenantID)
	if err != nil {
		return nil, err //nolint:wrapcheck // The store names the failed read.
	}
	if evaluation.Version == "" && evaluation.Digest == "" {
		return domain.CanonicalFile(map[string]any{})
	}
	return policy.BoundPolicy(evaluation.Version, evaluation.Digest)
}
