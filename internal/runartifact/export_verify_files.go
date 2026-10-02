package runartifact

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func verifyArtifactFiles(dir string) error {
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
	return nil
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
	return parseChecksumIndex(data)
}

func parseChecksumIndex(data []byte) (map[string]string, error) {
	expected := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		name, digest, err := parseChecksumEntry(scanner.Text())
		if err != nil {
			return nil, err
		}
		if _, duplicate := expected[name]; duplicate {
			return nil, fmt.Errorf("duplicate checksum entry for %s", name)
		}
		expected[name] = digest
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read checksums: %w", err)
	}
	return expected, nil
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
