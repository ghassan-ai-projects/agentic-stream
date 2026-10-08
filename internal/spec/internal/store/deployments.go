package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

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

func (r deploymentRecord) persist(ctx context.Context, tx *sql.Tx) error {
	if err := r.retirePriorVersions(ctx, tx); err != nil {
		return err
	}
	if err := registerInputSchemas(ctx, tx, r.compiled.Inputs, r.now); err != nil {
		return err
	}
	return r.insert(ctx, tx)
}

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
	record.now = kernel.FormatTime(time.Now())
	return record, nil
}

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

func (r deploymentRecord) retirePriorVersions(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE spec_deployments SET status = 'retired', activated_at = ?
		WHERE tenant_id = ? AND spec_name = ? AND status = 'active' AND deployment_id <> ?`,
		r.now, r.tenantID, r.compiled.Metadata.Name, r.compiled.Digest); err != nil {
		return fmt.Errorf("retire prior deployment: %w", err)
	}
	return nil
}

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

func LoadDeployment(ctx context.Context, db *storage.DB, deploymentID string) (*domain.CompiledSpec, error) {
	var compiledIR []byte
	if err := db.QueryRowContext(ctx, "SELECT compiled_ir FROM spec_deployments WHERE deployment_id = ?", deploymentID).Scan(&compiledIR); err != nil {
		return nil, fmt.Errorf("load deployment %s: %w", deploymentID, err)
	}
	var compiled domain.CompiledSpec
	if err := json.Unmarshal(compiledIR, &compiled); err != nil {
		return nil, fmt.Errorf("decode deployment %s: %w", deploymentID, err)
	}
	return &compiled, nil
}
