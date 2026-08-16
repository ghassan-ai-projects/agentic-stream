package spec

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventschema"
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

	specDigest, err := canonicaljson.DecodeDigest(compiled.Digest)
	if err != nil {
		return fmt.Errorf("decode compiled spec digest: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		// P8 (graph versioning): a definition change is a new version + a
		// FRESH namespace. Retire the previous active deployment of the same
		// name first (the schema enforces one-active-per-name), so the new
		// version's state never touches the old version's rows. A redeploy of
		// the SAME digest is idempotent — it must not retire itself.
		if _, err := tx.ExecContext(ctx, `
			UPDATE spec_deployments SET status = 'retired', activated_at = ?
			WHERE tenant_id = ? AND spec_name = ? AND status = 'active' AND deployment_id <> ?`,
			now, tenantID, compiled.Metadata.Name, compiled.Digest); err != nil {
			return fmt.Errorf("retire prior deployment: %w", err)
		}
		for _, input := range compiled.Inputs {
			definition, ok := eventschema.Lookup(input.SchemaRef)
			if !ok {
				continue
			}
			schemaJSON, err := eventschema.JSON(definition)
			if err != nil {
				return fmt.Errorf("build event schema %s: %w", input.SchemaRef, err)
			}
			if err := eventschema.Register(ctx, tx, definition, schemaJSON, now); err != nil {
				return fmt.Errorf("register event schema %s: %w", input.SchemaRef, err)
			}
		}
		_, err := tx.ExecContext(ctx, `
		INSERT INTO spec_deployments (
			deployment_id, tenant_id, spec_name, spec_version, spec_schema_version,
			spec_sha256, source_json, compiled_ir, status, activated_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?)
		ON CONFLICT(deployment_id) DO NOTHING`,
			compiled.Digest, tenantID, compiled.Metadata.Name, compiled.Metadata.Version,
			compiled.SchemaVersion, specDigest, sourceJSON, compiledIR, now, now)
		if err != nil {
			return fmt.Errorf("insert deployment: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("persist deployment: %w", err)
	}
	return nil
}
