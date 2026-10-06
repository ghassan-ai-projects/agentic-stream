package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// SaveDeployment persists a compiled spec as an active deployment record.
// It is idempotent: duplicate inserts for the same deployment_id are ignored.
func SaveDeployment(ctx context.Context, db *storage.DB, tenantID string, compiled *domain.CompiledSpec) error {
	record, err := newDeploymentRecord(tenantID, compiled)
	if err != nil {
		return err
	}
	if err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return record.persist(ctx, tx)
	}); err != nil {
		return fmt.Errorf("persist deployment: %w", err)
	}
	return nil
}

// persist retires earlier versions, registers the input schemas and inserts
// the deployment in one transaction.
func (r deploymentRecord) persist(ctx context.Context, tx *sql.Tx) error {
	if err := r.retirePriorVersions(ctx, tx); err != nil {
		return err
	}
	if err := registerInputSchemas(ctx, tx, r.compiled.Inputs, r.now); err != nil {
		return err
	}
	return r.insert(ctx, tx)
}

// deploymentRecord is a compiled spec in its stored form.
type deploymentRecord struct {
	tenantID               string
	compiled               *domain.CompiledSpec
	specDigest             []byte
	sourceJSON, compiledIR []byte
	now                    string
}

func newDeploymentRecord(tenantID string, compiled *domain.CompiledSpec) (deploymentRecord, error) {
	if compiled == nil {
		return deploymentRecord{}, fmt.Errorf("compiled spec is nil")
	}
	if compiled.Digest == "" {
		return deploymentRecord{}, fmt.Errorf("compiled spec digest is empty")
	}
	record := deploymentRecord{tenantID: tenantID, compiled: compiled, sourceJSON: compiled.CanonicalJSON}
	if err := record.encode(); err != nil {
		return deploymentRecord{}, err
	}
	record.now = time.Now().UTC().Format(time.RFC3339Nano)
	return record, nil
}

// encode fills the source JSON when the spec carries none, the compiled IR
// and the decoded spec digest.
func (r *deploymentRecord) encode() error {
	var err error
	if len(r.sourceJSON) == 0 {
		if r.sourceJSON, err = json.Marshal(r.compiled); err != nil {
			return fmt.Errorf("marshal compiled spec: %w", err)
		}
	}
	if r.compiledIR, err = json.Marshal(r.compiled); err != nil {
		return fmt.Errorf("marshal compiled ir: %w", err)
	}
	if r.specDigest, err = canonicaljson.DecodeDigest(r.compiled.Digest); err != nil {
		return fmt.Errorf("decode compiled spec digest: %w", err)
	}
	return nil
}

// retirePriorVersions implements P8 graph versioning: a definition change is
// a new version and a FRESH namespace. The previous active deployment of the
// same name is retired first (the schema enforces one active per name), so
// the new version's state never touches the old version's rows. A redeploy
// of the SAME digest is idempotent: it must not retire itself.
func (r deploymentRecord) retirePriorVersions(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE spec_deployments SET status = 'retired', activated_at = ?
		WHERE tenant_id = ? AND spec_name = ? AND status = 'active' AND deployment_id <> ?`,
		r.now, r.tenantID, r.compiled.Metadata.Name, r.compiled.Digest); err != nil {
		return fmt.Errorf("retire prior deployment: %w", err)
	}
	return nil
}

// registerInputSchemas registers the event schema of every input whose
// schema is known to the registry.
func registerInputSchemas(ctx context.Context, tx *sql.Tx, inputs []domain.Input, now string) error {
	for _, input := range inputs {
		definition, ok := domain.LookupEventSchema(input.SchemaRef)
		if !ok {
			continue
		}
		schemaJSON, err := domain.EventSchemaJSON(definition)
		if err != nil {
			return fmt.Errorf("build event schema %s: %w", input.SchemaRef, err)
		}
		if err := RegisterEventSchema(ctx, tx, definition, schemaJSON, now); err != nil {
			return fmt.Errorf("register event schema %s: %w", input.SchemaRef, err)
		}
	}
	return nil
}

// insert records the deployment as active; a repeated insert is ignored.
func (r deploymentRecord) insert(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO spec_deployments (
			deployment_id, tenant_id, spec_name, spec_version, spec_schema_version,
			spec_sha256, source_json, compiled_ir, status, activated_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?)
		ON CONFLICT(deployment_id) DO NOTHING`,
		r.compiled.Digest, r.tenantID, r.compiled.Metadata.Name, r.compiled.Metadata.Version,
		r.compiled.SchemaVersion, r.specDigest, r.sourceJSON, r.compiledIR, r.now, r.now); err != nil {
		return fmt.Errorf("insert deployment: %w", err)
	}
	return nil
}
