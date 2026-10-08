package runartifact_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestVerifyRejectsInvalidChecksumIndex(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, appendLine, want string
	}{
		{"missing hash", "manifest.json\n", "malformed checksum line"},
		{"invalid hex", strings.Repeat("g", 64) + "  extra.json\n", "malformed checksum for extra.json"},
		{"parent path", strings.Repeat("0", 64) + "  ../extra.json\n", "malformed checksum line"},
		{"backslash", strings.Repeat("0", 64) + "  nested\\extra.json\n", "malformed checksum line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := newVerificationArtifact(t)
			path := filepath.Join(dir, "checksums.sha256")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			writeVerificationFile(t, dir, "checksums.sha256", append(data, []byte(tt.appendLine)...))
			if err := runartifact.Verify(dir); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("verify = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestVerifyRejectsDuplicateChecksumEntry(t *testing.T) {
	t.Parallel()
	dir := newVerificationArtifact(t)
	path := filepath.Join(dir, "checksums.sha256")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	first, _, _ := strings.Cut(string(data), "\n")
	writeVerificationFile(t, dir, "checksums.sha256", append(data, []byte(first+"\n")...))
	if err := runartifact.Verify(dir); err == nil || !strings.Contains(err.Error(), "duplicate checksum entry") {
		t.Fatalf("duplicate checksum = %v", err)
	}
}

func TestVerifyPreservesIntegrityBeforeSyntaxValidation(t *testing.T) {
	t.Parallel()
	dir := newVerificationArtifact(t)
	// A valid checksum on invalid JSON must not bypass a mismatch elsewhere.
	rewriteVerifiedFile(t, dir, "manifest.json", []byte("invalid JSON"))
	writeVerificationFile(t, dir, "commands.jsonl", []byte("tampered"))
	if err := runartifact.Verify(dir); err == nil || !strings.Contains(err.Error(), "checksum mismatch for commands.jsonl") {
		t.Fatalf("verification precedence = %v", err)
	}
}

func TestVerifyRejectsSyntaxEvenWithMatchingChecksum(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, file, data, want string
	}{
		{"noncanonical JSON", "manifest.json", `{ "schema_version":1}`, "JSON is not canonical"},
		{"manifest version", "manifest.json", `{"schema_version":2}`, "unsupported manifest schema version 2"},
		{"blank record", "commands.jsonl", "\n", "blank JSONL record"},
		{"noncanonical record", "commands.jsonl", "{\"z\":2,\"a\":1}\n", "JSONL record is not canonical"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := newVerificationArtifact(t)
			rewriteVerifiedFile(t, dir, tt.file, []byte(tt.data))
			if err := runartifact.Verify(dir); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("verify = %v, want %q", err, tt.want)
			}
		})
	}
}

func newVerificationArtifact(t *testing.T) string {
	t.Helper()
	db := storagetest.OpenTemp(t)

	dir := filepath.Join(t.TempDir(), "artifact")
	if _, err := runartifact.Export(t.Context(), runartifact.Options{DB: db, OutputDir: dir, Manifest: runartifact.Manifest{TenantID: "tenant"}}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func rewriteVerifiedFile(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	path := filepath.Join(dir, name)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeVerificationFile(t, dir, name, data)
	checksumsPath := filepath.Join(dir, "checksums.sha256")
	checksums, err := os.ReadFile(checksumsPath)
	if err != nil {
		t.Fatal(err)
	}
	oldHash, newHash := sha256.Sum256(original), sha256.Sum256(data)
	oldLine := hex.EncodeToString(oldHash[:]) + "  " + name
	newLine := hex.EncodeToString(newHash[:]) + "  " + name
	if !strings.Contains(string(checksums), oldLine) {
		t.Fatalf("checksum fixture missing for %s", name)
	}
	writeVerificationFile(t, dir, "checksums.sha256", []byte(strings.Replace(string(checksums), oldLine, newLine, 1)))
}

func writeVerificationFile(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile(name, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
