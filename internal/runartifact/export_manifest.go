package runartifact

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func enrichManifest(ctx context.Context, tx *sql.Tx, input Manifest) (Manifest, error) {
	manifest := input
	applyManifestDefaults(&manifest)
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&manifest.MigrationVersion); err != nil {
		return Manifest{}, fmt.Errorf("read migration version: %w", err)
	}
	fillManifestDigests(ctx, tx, &manifest)
	return enrichDeviceIdentity(ctx, tx, manifest)
}

func applyManifestDefaults(manifest *Manifest) {
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
}

func fillManifestDigests(ctx context.Context, tx *sql.Tx, manifest *Manifest) {
	if manifest.SpecDigest == "" {
		var digest []byte
		if err := tx.QueryRowContext(ctx, "SELECT spec_sha256 FROM spec_deployments WHERE tenant_id = ? ORDER BY created_at DESC, deployment_id DESC LIMIT 1", manifest.TenantID).Scan(&digest); err == nil {
			manifest.SpecDigest = canonicaljson.EncodeDigest(digest)
		}
	}
	if manifest.PolicyDigest == "" {
		_ = tx.QueryRowContext(ctx, `SELECT pe.policy_digest FROM policy_evaluations pe
			JOIN intents i ON i.intent_id = pe.intent_id
			WHERE i.tenant_id = ? ORDER BY pe.evaluated_at DESC, pe.evaluation_id DESC LIMIT 1`, manifest.TenantID).Scan(&manifest.PolicyDigest)
	}
}

func enrichDeviceIdentity(ctx context.Context, tx *sql.Tx, manifest Manifest) (Manifest, error) {
	stateQuery, args, err := deviceStateQuery(ctx, tx, manifest.Device.DeviceID)
	if err != nil {
		return Manifest{}, err
	}
	var stateJSON []byte
	if err := tx.QueryRowContext(ctx, stateQuery, args...).Scan(&stateJSON); err != nil {
		return manifest, nil
	}
	var state map[string]any
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return manifest, nil
	}
	fillDeviceIdentity(&manifest.Device, state)
	return manifest, nil
}

func deviceStateQuery(ctx context.Context, tx *sql.Tx, deviceID string) (string, []any, error) {
	if deviceID != "" {
		return "SELECT state_json FROM device_reconciliation WHERE device_id = ?", []any{deviceID}, nil
	}
	if err := requireUnambiguousDevice(ctx, tx); err != nil {
		return "", nil, err
	}
	return "SELECT state_json FROM device_reconciliation ORDER BY device_id LIMIT 1", nil, nil
}

func requireUnambiguousDevice(ctx context.Context, tx *sql.Tx) error {
	var deviceCount int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM device_reconciliation").Scan(&deviceCount); err != nil {
		return fmt.Errorf("count device states: %w", err)
	}
	if deviceCount > 1 {
		return fmt.Errorf("manifest device_id is required when multiple device states exist")
	}
	return nil
}

func fillDeviceIdentity(device *DeviceIdentity, state map[string]any) {
	if device.DeviceID == "" {
		device.DeviceID, _ = state["device_id"].(string)
	}
	if device.BootID == "" {
		device.BootID, _ = state["boot_id"].(string)
	}
	if device.FirmwareDigest == "" {
		device.FirmwareDigest, _ = state["firmware_digest"].(string)
	}
	if device.CapabilityDigest == "" {
		device.CapabilityDigest, _ = state["capability_digest"].(string)
	}
}
