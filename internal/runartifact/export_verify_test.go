package runartifact_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestExportPublishesVerifiableArtifactAndRefusesImplicitOverwrite(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	output := filepath.Join(t.TempDir(), "artifact")
	path, err := runartifact.Export(t.Context(), runartifact.Options{
		DB: db, OutputDir: output,
		Manifest: runartifact.Manifest{RunID: "run-1", TenantID: "tenant", GitCommit: "abc", KnownBlindSpots: []string{"physical HIL not run"}},
	})
	if err != nil || path != output {
		t.Fatalf("export path=%q err=%v", path, err)
	}
	if err := runartifact.Verify(output); err != nil {
		t.Fatalf("verify exported artifact: %v", err)
	}
	if _, err := runartifact.Export(t.Context(), runartifact.Options{DB: db, OutputDir: output, Manifest: runartifact.Manifest{TenantID: "tenant"}}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("export over an existing artifact = %v, want already exists", err)
	}
}

func TestVerifyDetectsTamperedJSONL(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	output := filepath.Join(t.TempDir(), "artifact")
	if _, err := runartifact.Export(t.Context(), runartifact.Options{DB: db, OutputDir: output, Manifest: runartifact.Manifest{TenantID: "tenant"}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(output, "commands.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSuffix(string(data), "\n")+"\n{\"tampered\":true}\n"), 0o600); err != nil { //nolint:gosec // path is rooted in t.TempDir
		t.Fatal(err)
	}
	if err := runartifact.Verify(output); err == nil || !strings.Contains(err.Error(), "checksum mismatch for commands.jsonl") {
		t.Fatalf("a tampered ledger = %v, want checksum mismatch for commands.jsonl", err)
	}
}

func TestVerifyDetectsStaleDurableCommandDigest(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	if _, err := db.ExecContext(t.Context(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	commandJSON, err := canonicaljson.Marshal(map[string]any{"command_id": "cmd-stale", "operation": "safe_stop"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO commands
		(command_id, intent_id, tenant_id, effector_route, normalized_target,
		 idempotency_key, command_json, command_sha256, status, created_at, updated_at)
		VALUES ('cmd-stale', 'intent-missing', 'tenant', 'safe_stop', 'fan-01', ?, ?, ?, 'succeeded', ?, ?)`,
		make([]byte, 32), commandJSON, make([]byte, 32), "2026-08-29T12:00:00.000000000Z", "2026-08-29T12:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "artifact")
	if _, err := runartifact.Export(t.Context(), runartifact.Options{DB: db, OutputDir: output, Manifest: runartifact.Manifest{TenantID: "tenant"}}); err != nil {
		t.Fatal(err)
	}
	if err := runartifact.Verify(output); err == nil || !strings.Contains(err.Error(), "command_sha256 does not match command_json") {
		t.Fatalf("a stale durable command digest = %v, want command_sha256 does not match command_json", err)
	}
}

func TestVerifyDetectsStaleSafetyEventDigest(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	details := []byte(`{"reason":"test"}`)
	detailsHash := sha256.Sum256(details)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO device_safety_events
		(event_type, target, details_json, details_sha256, occurred_at)
		VALUES ('unsafe_output', 'fan-01', ?, ?, '2026-08-29T12:00:00.000000000Z')`, details, detailsHash[:]); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "artifact")
	if _, err := runartifact.Export(t.Context(), runartifact.Options{DB: db, OutputDir: output, Manifest: runartifact.Manifest{TenantID: "tenant"}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(output, "safety-events.jsonl")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := []byte(strings.Replace(string(original), "test", "tampered", 1))
	if err := os.WriteFile(path, tampered, 0o600); err != nil { //nolint:gosec // path is rooted in t.TempDir
		t.Fatal(err)
	}
	checksumsPath := filepath.Join(output, "checksums.sha256")
	checksums, err := os.ReadFile(checksumsPath)
	if err != nil {
		t.Fatal(err)
	}
	tamperedHash := sha256.Sum256(tampered)
	originalHash := sha256.Sum256(original)
	oldLine := fmt.Sprintf("%s  safety-events.jsonl", hex.EncodeToString(originalHash[:]))
	newLine := fmt.Sprintf("%s  safety-events.jsonl", hex.EncodeToString(tamperedHash[:]))
	updatedChecksums := []byte(strings.Replace(string(checksums), oldLine, newLine, 1))
	if string(updatedChecksums) == string(checksums) {
		t.Fatal("failed to update safety event checksum fixture")
	}
	if err := os.WriteFile(checksumsPath, updatedChecksums, 0o600); err != nil { //nolint:gosec // path is rooted in t.TempDir
		t.Fatal(err)
	}
	if err := runartifact.Verify(output); err == nil || !strings.Contains(err.Error(), "details_sha256 does not match details_json") {
		t.Fatalf("a stale safety event digest = %v, want details_sha256 does not match details_json", err)
	}
}
