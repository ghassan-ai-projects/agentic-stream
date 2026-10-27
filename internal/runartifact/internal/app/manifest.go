package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/store"
)

func enrichManifest(ctx context.Context, snapshot *store.Snapshot, input domain.Manifest) (domain.Manifest, error) {
	manifest := input.WithDefaults()
	version, err := snapshot.MigrationVersion(ctx)
	if err != nil {
		return domain.Manifest{}, err
	}
	manifest.MigrationVersion = version
	manifest, err = withRecordedDigests(ctx, snapshot, manifest)
	if err != nil {
		return domain.Manifest{}, err
	}
	if manifest.PlaybookDigests, err = recordedPlaybookDigests(ctx, snapshot, manifest); err != nil {
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
		return "", err
	}
	return canonicaljson.EncodeDigest(raw), nil
}

func recordedPlaybookDigests(ctx context.Context, snapshot *store.Snapshot, manifest domain.Manifest) ([]string, error) {
	if len(manifest.PlaybookDigests) > 0 {
		return manifest.PlaybookDigests, nil
	}
	return snapshot.PlaybookDigests(ctx, manifest.TenantID)
}

func recordedPolicyDigest(ctx context.Context, snapshot *store.Snapshot, manifest domain.Manifest) (string, error) {
	if manifest.PolicyDigest != "" {
		return "", nil
	}
	evaluation, err := snapshot.LatestPolicyEvaluation(ctx, manifest.TenantID)
	return evaluation.Digest, err
}

func withDeviceIdentity(ctx context.Context, snapshot *store.Snapshot, manifest domain.Manifest) (domain.Manifest, error) {
	if err := requireOneDevice(ctx, snapshot, manifest); err != nil {
		return domain.Manifest{}, err
	}
	raw, err := snapshot.DeviceState(ctx, manifest.Device.DeviceID)
	if err != nil {
		return domain.Manifest{}, err
	}
	if state, ok := domain.DecodeDeviceState(raw); ok {
		manifest = manifest.WithDeviceState(state)
	}
	return manifest, nil
}

func requireOneDevice(ctx context.Context, snapshot *store.Snapshot, manifest domain.Manifest) error {
	if manifest.Device.DeviceID != "" {
		return nil
	}
	count, err := snapshot.DeviceStateCount(ctx)
	if err != nil {
		return err
	}
	return domain.RequireUnambiguousDevice(count)
}
