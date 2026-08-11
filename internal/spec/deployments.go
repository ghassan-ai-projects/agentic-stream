package spec

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// SaveDeployment persists a compiled spec as an active deployment record.
// It is idempotent: duplicate inserts for the same deployment_id are ignored.
func SaveDeployment(ctx context.Context, db *storage.DB, tenantID string, compiled *CompiledSpec) error {
	if compiled == nil {
		return fmt.Errorf("compiled spec is nil")
	}
	if compiled.Digest == "" {
		return fmt.Errorf("compiled spec digest is empty")
	}

	sourceJSON := compiled.CanonicalJSON
	if len(sourceJSON) == 0 {
		var err error
		sourceJSON, err = json.Marshal(compiled)
		if err != nil {
			return fmt.Errorf("marshal compiled spec: %w", err)
		}
	}

	compiledIR, err := json.Marshal(compiled)
	if err != nil {
		return fmt.Errorf("marshal compiled ir: %w", err)
	}

	h := sha256.Sum256(sourceJSON)
	_, err = db.ExecContext(ctx, `
		INSERT INTO spec_deployments (
			deployment_id, tenant_id, spec_name, spec_version, spec_schema_version,
			spec_sha256, source_json, compiled_ir, status, activated_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?)
		ON CONFLICT(deployment_id) DO NOTHING`,
		compiled.Digest, tenantID, compiled.Metadata.Name, compiled.Metadata.Version,
		compiled.SchemaVersion, h[:], sourceJSON, compiledIR,
		time.Now().UTC().Format(time.RFC3339Nano),
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert deployment: %w", err)
	}
	return nil
}
