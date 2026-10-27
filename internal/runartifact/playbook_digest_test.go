package runartifact_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestExportBindsThePlaybookDigestsTheRunRecorded(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO rule_fires (fire_id, tenant_id, rule_id, rule_version, playbook_digest,
		episode_id, situation_id, situation_version, recommendation_json, audit_due, fired_at)
		VALUES ('rfire_e1', 'tenant', 'r', 1, 'sha256:pb', 'e1', 's1', 1, X'7B7D', 0, '2026-01-01T00:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "artifact")
	if _, err := runartifact.Export(t.Context(), runartifact.Options{DB: db, OutputDir: output, Manifest: runartifact.Manifest{TenantID: "tenant"}}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(output, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		PlaybookDigests []string `json:"playbook_digests"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil || !slices.Equal(manifest.PlaybookDigests, []string{"sha256:pb"}) {
		t.Fatalf("manifest playbook digests = %v (%v), want the recorded one", manifest.PlaybookDigests, err)
	}
	if err := runartifact.Verify(output); err != nil {
		t.Fatalf("verify: %v", err)
	}
}
