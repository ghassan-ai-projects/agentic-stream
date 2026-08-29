package runartifact

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
	"github.com/ghassan-ai-projects/agentic-stream/internal/soak"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func snapshot(ctx context.Context, db *storage.DB, input Manifest) (map[string][]byte, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin run artifact snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	manifest, err := enrichManifest(ctx, tx, input)
	if err != nil {
		return nil, fmt.Errorf("enrich run manifest: %w", err)
	}
	report, err := soak.ComputeTenantTx(ctx, tx, manifest.TenantID)
	if err != nil {
		return nil, fmt.Errorf("compute run soak report: %w", err)
	}

	files, err := exportLedgerFiles(ctx, tx, manifest.TenantID)
	if err != nil {
		return nil, err
	}
	for name, value := range map[string]any{
		"manifest.json": manifest,
		"metrics.json":  report,
		"verdict.json":  map[string]any{"verdict": report.Verdict, "failure_reasons": report.FailureReasons},
	} {
		encoded, err := canonicalJSONFile(value)
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", name, err)
		}
		files[name] = encoded
	}

	spec, err := queryCanonicalSpec(ctx, tx, manifest.TenantID)
	if err != nil {
		return nil, err
	}
	files["spec.canonical.json"] = spec
	policyData, err := queryCanonicalPolicy(ctx, tx, manifest.TenantID)
	if err != nil {
		return nil, err
	}
	files["policy.canonical.json"] = policyData
	if err := verifyManifestBindings(manifest, spec, policyData); err != nil {
		return nil, fmt.Errorf("verify manifest bindings: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit run artifact snapshot: %w", err)
	}
	return files, nil
}

func exportLedgerFiles(ctx context.Context, tx *sql.Tx, tenantID string) (map[string][]byte, error) {
	queries := map[string]struct {
		query string
		args  []any
	}{
		"observations.jsonl": {"SELECT * FROM event_log WHERE tenant_id = ? ORDER BY position", []any{tenantID}},
		"situations.jsonl": {`SELECT sv.* FROM situation_versions sv
			JOIN situations s ON s.situation_id = sv.situation_id
			WHERE s.tenant_id = ? ORDER BY sv.situation_id, sv.version`, []any{tenantID}},
		"decisions.jsonl": {`SELECT d.* FROM decisions d
			JOIN episodes e ON e.episode_id = d.episode_id
			WHERE e.tenant_id = ? ORDER BY d.decision_id`, []any{tenantID}},
		"commands.jsonl": {"SELECT * FROM commands WHERE tenant_id = ? ORDER BY command_id", []any{tenantID}},
		"device-results.jsonl": {`SELECT o.* FROM outcomes o
			JOIN commands c ON c.command_id = o.command_id
			WHERE c.tenant_id = ? ORDER BY o.command_id, o.ordinal`, []any{tenantID}},
		"feedback.jsonl": {`SELECT v.* FROM verifications v
			JOIN commands c ON c.command_id = v.command_id
			WHERE c.tenant_id = ? ORDER BY v.verification_id`, []any{tenantID}},
		"device-command-bindings.jsonl": {`SELECT b.* FROM device_command_bindings b
			JOIN commands c ON c.command_id = b.command_id
			WHERE c.tenant_id = ? ORDER BY b.command_id`, []any{tenantID}},
		"authority-events.jsonl": {"SELECT * FROM device_authority_events ORDER BY event_id", nil},
		"safety-events.jsonl":    {"SELECT * FROM device_safety_events ORDER BY event_id", nil},
	}
	files := make(map[string][]byte, len(queries))
	for name, item := range queries {
		data, err := queryJSONL(ctx, tx, item.query, item.args...)
		if err != nil {
			return nil, fmt.Errorf("export %s: %w", name, err)
		}
		files[name] = data
	}
	return files, nil
}

func enrichManifest(ctx context.Context, tx *sql.Tx, input Manifest) (Manifest, error) {
	manifest := input
	if manifest.SchemaVersion == 0 {
		manifest.SchemaVersion = artifactSchemaVersion
	}
	if manifest.KnownBlindSpots == nil {
		manifest.KnownBlindSpots = []string{
			"gateway transport and firmware provenance are external to Agentic Stream",
			"independent physical feedback verifier and HIL evidence are not stored by this repository",
			"current ledgers have no execution identifier; exported rows are a tenant snapshot",
			"process-local telemetry is diagnostic and is not part of the safety verdict",
		}
	}
	sort.Strings(manifest.KnownBlindSpots)
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&manifest.MigrationVersion); err != nil {
		return Manifest{}, fmt.Errorf("read migration version: %w", err)
	}
	if manifest.SpecDigest == "" {
		var digest []byte
		if err := tx.QueryRowContext(ctx, "SELECT spec_sha256 FROM spec_deployments WHERE tenant_id = ? ORDER BY created_at DESC, deployment_id DESC LIMIT 1", manifest.TenantID).Scan(&digest); err == nil {
			manifest.SpecDigest = "sha256:" + hex.EncodeToString(digest)
		}
	}
	if manifest.PolicyDigest == "" {
		_ = tx.QueryRowContext(ctx, `SELECT pe.policy_digest FROM policy_evaluations pe
			JOIN intents i ON i.intent_id = pe.intent_id
			WHERE i.tenant_id = ? ORDER BY pe.evaluated_at DESC, pe.evaluation_id DESC LIMIT 1`, manifest.TenantID).Scan(&manifest.PolicyDigest)
	}
	return enrichDeviceIdentity(ctx, tx, manifest)
}

func enrichDeviceIdentity(ctx context.Context, tx *sql.Tx, manifest Manifest) (Manifest, error) {
	stateQuery := "SELECT state_json FROM device_reconciliation ORDER BY device_id LIMIT 1"
	var args []any
	if manifest.Device.DeviceID != "" {
		stateQuery = "SELECT state_json FROM device_reconciliation WHERE device_id = ?"
		args = []any{manifest.Device.DeviceID}
	} else {
		var deviceCount int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM device_reconciliation").Scan(&deviceCount); err != nil {
			return Manifest{}, fmt.Errorf("count device states: %w", err)
		}
		if deviceCount > 1 {
			return Manifest{}, fmt.Errorf("manifest device_id is required when multiple device states exist")
		}
	}
	var stateJSON []byte
	if err := tx.QueryRowContext(ctx, stateQuery, args...).Scan(&stateJSON); err != nil {
		return manifest, nil
	}
	var state map[string]any
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return manifest, nil
	}
	if manifest.Device.DeviceID == "" {
		manifest.Device.DeviceID, _ = state["device_id"].(string)
	}
	if manifest.Device.BootID == "" {
		manifest.Device.BootID, _ = state["boot_id"].(string)
	}
	if manifest.Device.FirmwareDigest == "" {
		manifest.Device.FirmwareDigest, _ = state["firmware_digest"].(string)
	}
	if manifest.Device.CapabilityDigest == "" {
		manifest.Device.CapabilityDigest, _ = state["capability_digest"].(string)
	}
	return manifest, nil
}

func queryJSONL(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]byte, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query artifact rows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("read artifact columns: %w", err)
	}
	var out []byte
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, fmt.Errorf("scan artifact row: %w", err)
		}
		document := make(map[string]any, len(columns))
		for i, column := range columns {
			document[column] = databaseValue(values[i])
		}
		line, err := canonicaljson.Marshal(document)
		if err != nil {
			return nil, fmt.Errorf("canonicalize artifact row: %w", err)
		}
		out = append(out, line...)
		out = append(out, '\n')
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artifact rows: %w", err)
	}
	return out, nil
}

func databaseValue(value any) any {
	data, ok := value.([]byte)
	if !ok {
		return value
	}
	if json.Valid(data) {
		var decoded any
		if json.Unmarshal(data, &decoded) == nil {
			return decoded
		}
	}
	return base64.StdEncoding.EncodeToString(data)
}

func queryCanonicalSpec(ctx context.Context, tx *sql.Tx, tenantID string) ([]byte, error) {
	var source []byte
	if err := tx.QueryRowContext(ctx, "SELECT source_json FROM spec_deployments WHERE tenant_id = ? ORDER BY created_at DESC, deployment_id DESC LIMIT 1", tenantID).Scan(&source); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return canonicalJSONFile(map[string]any{})
		}
		return nil, fmt.Errorf("read canonical spec: %w", err)
	}
	return canonicalizeStoredJSON(source)
}

func queryCanonicalPolicy(ctx context.Context, tx *sql.Tx, tenantID string) ([]byte, error) {
	var version, digest string
	err := tx.QueryRowContext(ctx, `SELECT policy_version, policy_digest
		FROM policy_evaluations pe JOIN intents i ON i.intent_id = pe.intent_id
		WHERE i.tenant_id = ? ORDER BY pe.evaluated_at DESC, pe.evaluation_id DESC LIMIT 1`, tenantID).Scan(&version, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return canonicalJSONFile(map[string]any{})
	}
	if err != nil {
		return nil, fmt.Errorf("read current policy input: %w", err)
	}
	expected, err := policy.DigestForVersion(version)
	if err != nil {
		return nil, fmt.Errorf("digest current policy input: %w", err)
	}
	if expected != digest {
		return nil, fmt.Errorf("policy evaluation digest does not match policy version %q", version)
	}
	return canonicalJSONFile(policy.CanonicalDocumentForVersion(version))
}

func verifyManifestBindings(manifest Manifest, specData, policyData []byte) error {
	if manifest.SpecDigest != "" {
		var specDocument any
		if err := json.Unmarshal(bytesTrimSpace(specData), &specDocument); err != nil {
			return fmt.Errorf("decode canonical spec: %w", err)
		}
		expected, err := canonicaljson.Digest(canonicaljson.DomainSpec, specDocument)
		if err != nil {
			return fmt.Errorf("digest canonical spec: %w", err)
		}
		if expected != manifest.SpecDigest {
			return fmt.Errorf("manifest spec digest %q does not match canonical spec %q", manifest.SpecDigest, expected)
		}
	}
	if manifest.PolicyDigest != "" {
		var policyDocument map[string]any
		if err := json.Unmarshal(bytesTrimSpace(policyData), &policyDocument); err != nil {
			return fmt.Errorf("decode canonical policy: %w", err)
		}
		version, _ := policyDocument["policy_version"].(string)
		expected, err := policy.DigestForVersion(version)
		if err != nil {
			return fmt.Errorf("digest canonical policy: %w", err)
		}
		if expected != manifest.PolicyDigest {
			return fmt.Errorf("manifest policy digest %q does not match canonical policy %q", manifest.PolicyDigest, expected)
		}
	}
	return nil
}
