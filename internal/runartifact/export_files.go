package runartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func canonicalizeStoredJSON(data []byte) ([]byte, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("stored JSON is invalid: %w", err)
	}
	return canonicalJSONFile(value)
}

func canonicalJSONFile(value any) ([]byte, error) {
	data, err := canonicaljson.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("canonicalize artifact JSON: %w", err)
	}
	return append(data, '\n'), nil
}

func checksumFile(files map[string][]byte) ([]byte, error) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []byte
	for _, name := range names {
		hash := sha256.Sum256(files[name])
		out = append(out, fmt.Sprintf("%s  %s\n", hex.EncodeToString(hash[:]), name)...)
	}
	return out, nil
}

func writeFile(dir, name string, data []byte) error {
	if filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid artifact file name %q", name)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

func bytesTrimSpace(data []byte) []byte { return []byte(strings.TrimSpace(string(data))) }
