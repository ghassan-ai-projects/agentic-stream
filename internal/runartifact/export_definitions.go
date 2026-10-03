package runartifact

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy"
)

// addBoundDefinitions adds the canonical spec and policy and requires the
// manifest to bind both.
func addBoundDefinitions(ctx context.Context, tx *sql.Tx, manifest Manifest, files map[string][]byte) error {
	spec, err := queryCanonicalSpec(ctx, tx, manifest.TenantID)
	if err != nil {
		return err
	}
	files["spec.canonical.json"] = spec
	policyData, err := queryCanonicalPolicy(ctx, tx, manifest.TenantID)
	if err != nil {
		return err
	}
	files["policy.canonical.json"] = policyData
	if err := verifyManifestBindings(manifest, spec, policyData); err != nil {
		return fmt.Errorf("verify manifest bindings: %w", err)
	}
	return nil
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
	return boundCanonicalPolicy(version, digest)
}

func boundCanonicalPolicy(version, digest string) ([]byte, error) {
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
	if err := verifySpecBinding(manifest.SpecDigest, specData); err != nil {
		return err
	}
	return verifyPolicyBinding(manifest.PolicyDigest, policyData)
}

func verifySpecBinding(digest string, data []byte) error {
	if digest == "" {
		return nil
	}
	var document any
	if err := json.Unmarshal(bytesTrimSpace(data), &document); err != nil {
		return fmt.Errorf("decode canonical spec: %w", err)
	}
	expected, err := canonicaljson.Digest(canonicaljson.DomainSpec, document)
	if err != nil {
		return fmt.Errorf("digest canonical spec: %w", err)
	}
	if expected != digest {
		return fmt.Errorf("manifest spec digest %q does not match canonical spec %q", digest, expected)
	}
	return nil
}

func verifyPolicyBinding(digest string, data []byte) error {
	if digest == "" {
		return nil
	}
	var document map[string]any
	if err := json.Unmarshal(bytesTrimSpace(data), &document); err != nil {
		return fmt.Errorf("decode canonical policy: %w", err)
	}
	return verifyPolicyDocumentDigest(digest, document)
}

func verifyPolicyDocumentDigest(digest string, document map[string]any) error {
	version, _ := document["policy_version"].(string)
	expected, err := policy.DigestForVersion(version)
	if err != nil {
		return fmt.Errorf("digest canonical policy: %w", err)
	}
	if expected != digest {
		return fmt.Errorf("manifest policy digest %q does not match canonical policy %q", digest, expected)
	}
	return nil
}
