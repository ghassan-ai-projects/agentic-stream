package app

import (
	"context"
	"fmt"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/transport"
)

// Request is one export: the run store, a new output directory, the
// operator's manifest and the policy documents the manifest binds to.
type Request struct {
	Store     store.Store
	OutputDir string
	Manifest  domain.Manifest
	Policy    domain.PolicyDocuments
}

// Export creates an immutable run directory and returns its absolute path. It
// never overwrites an existing directory.
func Export(ctx context.Context, request Request) (string, error) {
	if err := validateRequest(request); err != nil {
		return "", err
	}
	output, err := transport.ReserveOutput(request.OutputDir)
	if err != nil {
		return "", err
	}
	files, err := snapshotFiles(ctx, request)
	if err != nil {
		return "", err
	}
	if err := transport.Publish(output, files); err != nil {
		return "", err
	}
	return output, nil
}

func validateRequest(request Request) error {
	if request.Store.IsZero() || request.OutputDir == "" {
		return fmt.Errorf("run artifact database and output directory are required")
	}
	if request.Manifest.TenantID == "" {
		return fmt.Errorf("run artifact tenant is required")
	}
	return nil
}

func snapshotFiles(ctx context.Context, request Request) (map[string][]byte, error) {
	var files map[string][]byte
	err := request.Store.InSnapshot(ctx, func(snapshot *store.Snapshot) error {
		built, err := buildFiles(ctx, snapshot, request)
		files = built
		return err
	})
	return files, err //nolint:wrapcheck // The store names the failed snapshot step.
}

// buildFiles gathers the ledgers, the soak report and the bound definitions.
func buildFiles(ctx context.Context, snapshot *store.Snapshot, request Request) (map[string][]byte, error) {
	manifest, err := enrichManifest(ctx, snapshot, request.Manifest)
	if err != nil {
		return nil, fmt.Errorf("enrich run manifest: %w", err)
	}
	files, err := ledgerFiles(ctx, snapshot, manifest.TenantID)
	if err != nil {
		return nil, err
	}
	if err := addReportFiles(ctx, snapshot, manifest, files); err != nil {
		return nil, err
	}
	if err := addBoundDefinitions(ctx, snapshot, manifest, request.Policy, files); err != nil {
		return nil, err
	}
	return files, nil
}
