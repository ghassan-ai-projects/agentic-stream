package runartifact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Verify checks all checksums, expected files, and JSON/JSONL syntax in a run
// directory. It does not trust the manifest to define its own integrity.
func Verify(dir string) error {
	if dir == "" {
		return fmt.Errorf("run artifact directory is required")
	}
	if err := verifyArtifactFiles(dir); err != nil {
		return err
	}
	if err := verifyJSONFiles(dir); err != nil {
		return err
	}
	if err := verifyArtifactManifest(dir); err != nil {
		return err
	}
	return verifyLedgerFiles(dir)
}

func verifyArtifactManifest(dir string) error {
	manifest, err := readManifest(dir)
	if err != nil {
		return err
	}
	specData, err := readArtifactFile(dir, "spec.canonical.json")
	if err != nil {
		return err
	}
	policyData, err := readArtifactFile(dir, "policy.canonical.json")
	if err != nil {
		return err
	}
	if err := verifyManifestBindings(manifest, specData, policyData); err != nil {
		return fmt.Errorf("verify manifest bindings: %w", err)
	}
	return nil
}

func readManifest(dir string) (Manifest, error) {
	data, err := readArtifactFile(dir, "manifest.json")
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest.json: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(bytesTrimSpace(data), &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.SchemaVersion != artifactSchemaVersion {
		return Manifest{}, fmt.Errorf("unsupported manifest schema version %d", manifest.SchemaVersion)
	}
	return manifest, nil
}

func readArtifactFile(dir, name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(dir, name)) //nolint:wrapcheck // callers add the operation-specific file context.
}
