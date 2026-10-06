package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/store"
)

// enrichManifest completes the operator's manifest from the snapshot: schema
// defaults, migration version, recorded digests and device identity.
func enrichManifest(ctx context.Context, snapshot *store.Snapshot, input domain.Manifest) (domain.Manifest, error) {
	manifest := input.WithDefaults()
	version, err := snapshot.MigrationVersion(ctx)
	if err != nil {
		return domain.Manifest{}, err //nolint:wrapcheck // The store names the failed read.
	}
	manifest.MigrationVersion = version
	manifest, err = withRecordedDigests(ctx, snapshot, manifest)
	if err != nil {
		return domain.Manifest{}, err
	}
	return withDeviceIdentity(ctx, snapshot, manifest)
}

func withRecordedDigests(ctx context.Context, snapshot *store.Snapshot, manifest domain.Manifest) (domain.Manifest, error) {
	specDigest, err := recordedSpecDigest(ctx, snapshot, manifest)
	if err != nil {
		return domain.Manifest{}, err
	}
	policyDigest, err := recordedPolicyDigest(ctx, snapshot, manifest)
	if err != nil {
		return domain.Manifest{}, err
	}
	return manifest.WithRecordedDigests(specDigest, policyDigest), nil
}

func recordedSpecDigest(ctx context.Context, snapshot *store.Snapshot, manifest domain.Manifest) (string, error) {
	if manifest.SpecDigest != "" {
		return "", nil
	}
	raw, err := snapshot.LatestSpecDigest(ctx, manifest.TenantID)
	if err != nil || len(raw) == 0 {
		return "", err //nolint:wrapcheck // The store names the failed read.
	}
	return canonicaljson.EncodeDigest(raw), nil
}

func recordedPolicyDigest(ctx context.Context, snapshot *store.Snapshot, manifest domain.Manifest) (string, error) {
	if manifest.PolicyDigest != "" {
		return "", nil
	}
	evaluation, err := snapshot.LatestPolicyEvaluation(ctx, manifest.TenantID)
	return evaluation.Digest, err //nolint:wrapcheck // The store names the failed read.
}

func withDeviceIdentity(ctx context.Context, snapshot *store.Snapshot, manifest domain.Manifest) (domain.Manifest, error) {
	if manifest.Device.DeviceID == "" {
		count, err := snapshot.DeviceStateCount(ctx)
		if err != nil {
			return domain.Manifest{}, err //nolint:wrapcheck // The store names the failed read.
		}
		if err := domain.RequireUnambiguousDevice(count); err != nil {
			return domain.Manifest{}, err //nolint:wrapcheck // The domain rule's message is the operator-facing text.
		}
	}
	raw, err := snapshot.DeviceState(ctx, manifest.Device.DeviceID)
	if err != nil {
		return domain.Manifest{}, err //nolint:wrapcheck // The store names the failed read.
	}
	if state, ok := domain.DecodeDeviceState(raw); ok {
		manifest = manifest.WithDeviceState(state)
	}
	return manifest, nil
}
