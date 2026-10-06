package app

import (
	"context"
	"fmt"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/store"
)

// ledgerFiles encodes every ledger of the snapshot as canonical JSON Lines.
func ledgerFiles(ctx context.Context, snapshot *store.Snapshot, tenantID string) (map[string][]byte, error) {
	files := make(map[string][]byte)
	for _, name := range domain.LedgerFiles() {
		table, err := snapshot.Ledger(ctx, name, tenantID)
		if err != nil {
			return nil, fmt.Errorf("export %s: %w", name, err)
		}
		if files[name], err = domain.EncodeLedger(table); err != nil {
			return nil, fmt.Errorf("export %s: %w", name, err)
		}
	}
	return files, nil
}

// addReportFiles adds the manifest, the soak metrics and the verdict.
func addReportFiles(ctx context.Context, snapshot *store.Snapshot, manifest domain.Manifest, files map[string][]byte) error {
	evidence, err := snapshot.SafetyEvidence(ctx, manifest.TenantID)
	if err != nil {
		return fmt.Errorf("compute run soak report: %w", err)
	}
	report := domain.DeriveSoakReport(evidence)
	return encodeInto(files, map[string]any{
		domain.FileManifest: manifest, domain.FileMetrics: report,
		domain.FileVerdict: map[string]any{"verdict": report.Verdict, "failure_reasons": report.FailureReasons},
	})
}

func encodeInto(files map[string][]byte, documents map[string]any) error {
	for name, value := range documents {
		encoded, err := domain.CanonicalFile(value)
		if err != nil {
			return fmt.Errorf("encode %s: %w", name, err)
		}
		files[name] = encoded
	}
	return nil
}
