package domain

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

// ChecksumsFile is the integrity index of an artifact directory. It is the only
// file the index does not list.
const ChecksumsFile = "checksums.sha256"

// Artifact file names. Ledger files are JSON Lines, document files are one
// canonical JSON value.
const (
	FileObservations = "observations.jsonl"
	FileSituations   = "situations.jsonl"
	FileDecisions    = "decisions.jsonl"
	FileCommands     = "commands.jsonl"
	FileResults      = "device-results.jsonl"
	FileFeedback     = "feedback.jsonl"
	FileBindings     = "device-command-bindings.jsonl"
	FileAuthority    = "authority-events.jsonl"
	FileSafety       = "safety-events.jsonl"
	FileManifest     = "manifest.json"
	FileSpec         = "spec.canonical.json"
	FilePolicy       = "policy.canonical.json"
	FileMetrics      = "metrics.json"
	FileVerdict      = "verdict.json"
)

// LedgerFiles lists the JSON Lines files in verification order.
func LedgerFiles() []string {
	return []string{FileObservations, FileSituations, FileDecisions, FileCommands, FileResults, FileFeedback, FileBindings, FileAuthority, FileSafety}
}

// DocumentFiles lists the single-document JSON files in verification order.
func DocumentFiles() []string {
	return []string{FileManifest, FileSpec, FilePolicy, FileMetrics, FileVerdict}
}

// CanonicalFile encodes a value as one canonical JSON line.
func CanonicalFile(value any) ([]byte, error) {
	data, err := canonicaljson.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("canonicalize artifact JSON: %w", err)
	}
	return append(data, '\n'), nil
}

// CanonicalizeStored re-encodes stored JSON in canonical form.
func CanonicalizeStored(data []byte) ([]byte, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("stored JSON is invalid: %w", err)
	}
	return CanonicalFile(value)
}

// ChecksumIndex renders the checksum file for the given files, sorted by name.
func ChecksumIndex(files map[string][]byte) []byte {
	var out []byte
	for _, name := range slices.Sorted(maps.Keys(files)) {
		hash := sha256.Sum256(files[name])
		out = append(out, fmt.Sprintf("%s  %s\n", hex.EncodeToString(hash[:]), name)...)
	}
	return out
}

// RequireFileName refuses names that could escape the artifact directory.
func RequireFileName(name string) error {
	if filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid artifact file name %q", name)
	}
	return nil
}

// ParseChecksumIndex reads the checksum file into name-to-digest entries and
// refuses malformed or duplicate lines.
func ParseChecksumIndex(data []byte) (map[string]string, error) {
	expected := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		if err := IndexChecksumEntry(expected, scanner.Text()); err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read checksums: %w", err)
	}
	return expected, nil
}

// IndexChecksumEntry adds one checksum line; a duplicate never replaces the
// first digest.
func IndexChecksumEntry(expected map[string]string, line string) error {
	name, digest, err := parseChecksumEntry(line)
	if err != nil {
		return err
	}
	if _, duplicate := expected[name]; duplicate {
		return fmt.Errorf("duplicate checksum entry for %s", name)
	}
	expected[name] = digest
	return nil
}

func parseChecksumEntry(line string) (string, string, error) {
	fields := strings.Fields(line)
	if len(fields) != 2 || len(fields[0]) != sha256.Size*2 || fields[1] == "." || fields[1] == ".." || filepath.Base(fields[1]) != fields[1] || strings.Contains(fields[1], "\\") {
		return "", "", fmt.Errorf("malformed checksum line %q", line)
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return "", "", fmt.Errorf("malformed checksum for %s: %w", fields[1], err)
	}
	return fields[1], fields[0], nil
}

// RequireExpectedFiles refuses an index that omits a file every artifact has.
func RequireExpectedFiles(expected map[string]string) error {
	for _, name := range append(LedgerFiles(), DocumentFiles()...) {
		if _, ok := expected[name]; !ok {
			return fmt.Errorf("checksum missing for %s", name)
		}
	}
	return nil
}

// VerifyChecksum compares a file's SHA-256 with the indexed digest.
func VerifyChecksum(name string, data []byte, wanted string) error {
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != wanted {
		return fmt.Errorf("checksum mismatch for %s", name)
	}
	return nil
}

// RequireOnlyIndexedFiles refuses a directory entry the index does not list.
func RequireOnlyIndexedFiles(entries []string, expected map[string]string) error {
	for _, name := range entries {
		if _, ok := expected[name]; !ok && name != ChecksumsFile {
			return fmt.Errorf("unexpected file in run artifact: %s", name)
		}
	}
	return nil
}

func trimmed(data []byte) []byte { return []byte(strings.TrimSpace(string(data))) }
