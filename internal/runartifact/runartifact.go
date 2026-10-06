package runartifact

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	app "github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/app"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Manifest is the operator-supplied and database-derived provenance header of
// a run artifact.
type Manifest = domain.Manifest

// DeviceIdentity identifies the physical or emulated device state in the run.
type DeviceIdentity = domain.DeviceIdentity

// WorkerMetadata is the non-secret worker provenance needed to interpret a
// decision.
type WorkerMetadata = domain.WorkerMetadata

// Options controls one export. Export never overwrites an existing directory;
// callers must choose a new output path for a new immutable artifact.
type Options struct {
	DB        *storage.DB
	OutputDir string
	Manifest  Manifest
}

// Export creates an immutable run directory and returns its absolute path.
func Export(ctx context.Context, options Options) (string, error) {
	return app.Export(ctx, app.Request{Store: store.New(options.DB), OutputDir: options.OutputDir, Manifest: options.Manifest, Policy: policyDocuments()})
}

// Verify checks all checksums, expected files, JSON syntax, manifest bindings
// and ledger digests in a run directory.
func Verify(dir string) error {
	return app.Verify(dir, policyDocuments())
}

// policyDocuments hands the policy module's pure digest and definition
// functions to the layers below, which may not import policy.
func policyDocuments() domain.PolicyDocuments {
	return domain.PolicyDocuments{DigestForVersion: policy.DigestForVersion, Canonical: policy.CanonicalDocumentForVersion}
}
