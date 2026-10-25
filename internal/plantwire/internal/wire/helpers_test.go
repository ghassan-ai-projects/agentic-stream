package wire_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const contractDir = "plantwire-contract"

func fixtureFiles(t *testing.T, folder, pattern string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(contractDir, "fixtures", folder, pattern))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures in %s: %v", folder, err)
	}
	return files
}

func readDocument(t *testing.T, file string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	return decodeDocument(t, raw)
}

func decodeDocument(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return document
}

func lineOf(t *testing.T, document any) []byte {
	t.Helper()
	line, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return line
}

func exchangeSteps(t *testing.T, file string) (map[string]any, []map[string]any) {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	var steps []map[string]any
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		steps = append(steps, decodeDocument(t, scanner.Bytes()))
	}
	return steps[0], steps[1:]
}
