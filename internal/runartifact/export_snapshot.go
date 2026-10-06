package runartifact

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func snapshot(ctx context.Context, db *storage.DB, input Manifest) (map[string][]byte, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin run artifact snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	files, err := buildSnapshotFiles(ctx, tx, input)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit run artifact snapshot: %w", err)
	}
	return files, nil
}

func buildSnapshotFiles(ctx context.Context, tx *sql.Tx, input Manifest) (map[string][]byte, error) {
	manifest, err := enrichManifest(ctx, tx, input)
	if err != nil {
		return nil, fmt.Errorf("enrich run manifest: %w", err)
	}
	files, err := exportLedgerFiles(ctx, tx, manifest.TenantID)
	if err != nil {
		return nil, err
	}
	if err := addReportFiles(ctx, tx, manifest, files); err != nil {
		return nil, err
	}
	if err := addBoundDefinitions(ctx, tx, manifest, files); err != nil {
		return nil, err
	}
	return files, nil
}

// addReportFiles adds the manifest, the soak metrics, and the verdict.
func addReportFiles(ctx context.Context, tx *sql.Tx, manifest Manifest, files map[string][]byte) error {
	report, err := computeSoakReport(ctx, tx, manifest.TenantID)
	if err != nil {
		return fmt.Errorf("compute run soak report: %w", err)
	}
	return encodeReportFiles(files, map[string]any{
		"manifest.json": manifest, "metrics.json": report,
		"verdict.json": map[string]any{"verdict": report.Verdict, "failure_reasons": report.FailureReasons},
	})
}

func encodeReportFiles(files map[string][]byte, reports map[string]any) error {
	for name, value := range reports {
		encoded, err := canonicalJSONFile(value)
		if err != nil {
			return fmt.Errorf("encode %s: %w", name, err)
		}
		files[name] = encoded
	}
	return nil
}
