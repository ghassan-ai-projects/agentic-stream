package runartifact

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// Verify checks all checksums, expected files, and JSON/JSONL syntax in a run
// directory. It does not trust the manifest to define its own integrity.
func Verify(dir string) error {
	if dir == "" {
		return fmt.Errorf("run artifact directory is required")
	}
	if err := requireDirectory(dir); err != nil {
		return err
	}
	expected, err := readChecksums(dir)
	if err != nil {
		return err
	}
	if err := verifyExpectedFiles(dir, expected); err != nil {
		return err
	}
	if err := verifyDirectoryContents(dir, expected); err != nil {
		return err
	}
	if err := verifyJSONFiles(dir); err != nil {
		return err
	}
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
	return verifyLedgerFiles(dir)
}

func requireDirectory(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat run artifact: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("run artifact is not a directory")
	}
	return nil
}

func readChecksums(dir string) (map[string]string, error) {
	data, err := readArtifactFile(dir, "checksums.sha256")
	if err != nil {
		return nil, fmt.Errorf("read run artifact checksums: %w", err)
	}
	expected := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 || fields[1] == "." || fields[1] == ".." || filepath.Base(fields[1]) != fields[1] || strings.Contains(fields[1], "\\") {
			return nil, fmt.Errorf("malformed checksum line %q", scanner.Text())
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			return nil, fmt.Errorf("malformed checksum for %s: %w", fields[1], err)
		}
		if _, duplicate := expected[fields[1]]; duplicate {
			return nil, fmt.Errorf("duplicate checksum entry for %s", fields[1])
		}
		expected[fields[1]] = fields[0]
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read checksums: %w", err)
	}
	return expected, nil
}

func verifyExpectedFiles(dir string, expected map[string]string) error {
	for _, name := range append(append([]string{}, exportedJSONL...), exportedJSON...) {
		if _, ok := expected[name]; !ok {
			return fmt.Errorf("checksum missing for %s", name)
		}
	}
	for name, wanted := range expected {
		data, err := readArtifactFile(dir, name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		hash := sha256.Sum256(data)
		if hex.EncodeToString(hash[:]) != wanted {
			return fmt.Errorf("checksum mismatch for %s", name)
		}
	}
	return nil
}

func verifyDirectoryContents(dir string, expected map[string]string) error {
	allowed := make(map[string]struct{}, len(expected)+1)
	for name := range expected {
		allowed[name] = struct{}{}
	}
	allowed["checksums.sha256"] = struct{}{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read run artifact directory: %w", err)
	}
	for _, entry := range entries {
		if _, ok := allowed[entry.Name()]; !ok {
			return fmt.Errorf("unexpected file in run artifact: %s", entry.Name())
		}
	}
	return nil
}

func verifyJSONFiles(dir string) error {
	for _, name := range exportedJSON {
		data, err := readArtifactFile(dir, name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := verifyJSON(data); err != nil {
			return fmt.Errorf("verify %s: %w", name, err)
		}
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

func verifyJSON(data []byte) error {
	var value any
	if err := json.Unmarshal(bytesTrimSpace(data), &value); err != nil {
		return fmt.Errorf("decode stored JSON: %w", err)
	}
	canonical, err := canonicaljson.Marshal(value)
	if err != nil {
		return fmt.Errorf("canonicalize stored JSON: %w", err)
	}
	if string(canonical) != string(bytesTrimSpace(data)) {
		return fmt.Errorf("JSON is not canonical")
	}
	return nil
}

func verifyJSONL(data []byte) error {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 64*1024), maxJSONLLineBytes)
	for scanner.Scan() {
		if len(bytesTrimSpace(scanner.Bytes())) == 0 {
			return fmt.Errorf("blank JSONL record")
		}
		var value any
		if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
			return fmt.Errorf("decode JSONL record: %w", err)
		}
		canonical, err := canonicaljson.Marshal(value)
		if err != nil {
			return fmt.Errorf("canonicalize JSONL record: %w", err)
		}
		if string(canonical) != string(bytesTrimSpace(scanner.Bytes())) {
			return fmt.Errorf("JSONL record is not canonical")
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan JSONL: %w", err)
	}
	return nil
}
