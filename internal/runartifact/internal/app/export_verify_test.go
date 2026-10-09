package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/runartifact/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func stubPolicy() domain.PolicyDocuments {
	return domain.PolicyDocuments{
		DigestForVersion: func(string) (string, error) { return "sha256:policy", nil },
		Canonical:        func(version string) map[string]any { return map[string]any{"policy_version": version} },
	}
}

func openRun(t *testing.T) *storage.DB {
	t.Helper()
	db := storagetest.OpenTemp(t)

	return db
}

func TestExportThenVerifyRoundTripsAndRefusesOverwrite(t *testing.T) {
	t.Parallel()
	db := openRun(t)
	request := app.Request{Store: store.New(db), OutputDir: filepath.Join(t.TempDir(), "artifact"), Manifest: domain.Manifest{TenantID: "tenant"}, Policy: stubPolicy()}
	output, err := app.Export(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Verify(output, stubPolicy()); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if _, err := app.Export(t.Context(), request); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("overwrite = %v", err)
	}
}

func TestExportRequiresDatabaseOutputAndTenant(t *testing.T) {
	t.Parallel()
	db := openRun(t)
	for name, tc := range map[string]struct {
		request app.Request
		wantErr string
	}{
		"store":  {app.Request{OutputDir: "x", Manifest: domain.Manifest{TenantID: "t"}}, "database and output directory are required"},
		"output": {app.Request{Store: store.New(db), Manifest: domain.Manifest{TenantID: "t"}}, "database and output directory are required"},
		"tenant": {app.Request{Store: store.New(db), OutputDir: "x"}, "tenant is required"},
	} {
		if _, err := app.Export(t.Context(), tc.request); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("export without %s = %v, want %q", name, err, tc.wantErr)
		}
	}
}

func TestVerifyDetectsTamperingAndMissingDirectory(t *testing.T) {
	t.Parallel()
	db := openRun(t)
	output, err := app.Export(t.Context(), app.Request{Store: store.New(db), OutputDir: filepath.Join(t.TempDir(), "artifact"), Manifest: domain.Manifest{TenantID: "tenant"}, Policy: stubPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, domain.FileCommands), []byte("tampered"), 0o600); err != nil { //nolint:gosec // rooted in t.TempDir
		t.Fatal(err)
	}
	if err := app.Verify(output, stubPolicy()); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("tamper = %v", err)
	}
	if err := app.Verify("", stubPolicy()); err == nil || !strings.Contains(err.Error(), "run artifact directory is required") {
		t.Fatalf("an empty directory name = %v, want run artifact directory is required", err)
	}
	if err := app.Verify(filepath.Join(t.TempDir(), "missing"), stubPolicy()); err == nil || !strings.Contains(err.Error(), "stat run artifact") {
		t.Fatalf("a missing directory = %v, want stat run artifact", err)
	}
}
