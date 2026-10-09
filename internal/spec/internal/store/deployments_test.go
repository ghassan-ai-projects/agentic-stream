package store_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func digestOf(n int) string {
	return fmt.Sprintf("sha256:%064x", n)
}

func versionedSpec(name, version, digest string) *domain.CompiledSpec {
	return &domain.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Metadata:      domain.Metadata{Name: name, Version: version},
		CanonicalJSON: []byte(`{"kind":"SituationSpec"}`),
		Digest:        digest,
	}
}

func deploymentStatus(t *testing.T, db *storage.DB, tenant, digest string) string {
	t.Helper()
	var status string
	if err := db.QueryRowContext(context.Background(),
		"SELECT status FROM spec_deployments WHERE tenant_id = ? AND deployment_id = ?", tenant, digest).Scan(&status); err != nil {
		t.Fatalf("status of %s: %v", digest, err)
	}
	return status
}

func TestSaveDeploymentStoresCanonicalDigestBytes(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	digest := "sha256:" + strings.Repeat("ab", 32)

	if err := store.SaveDeployment(t.Context(), db, "default", versionedSpec("test", "v1", digest)); err != nil {
		t.Fatalf("save deployment: %v", err)
	}

	var got []byte
	if err := db.QueryRowContext(t.Context(), "SELECT spec_sha256 FROM spec_deployments WHERE deployment_id = ?", digest).Scan(&got); err != nil {
		t.Fatalf("query stored digest: %v", err)
	}
	want, err := canonicaljson.DecodeDigest(digest)
	if err != nil {
		t.Fatalf("decode expected digest: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stored digest = %x, want %x", got, want)
	}
}

func TestSaveDeploymentRefusesASpecItCannotIdentify(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		compiled *domain.CompiledSpec
		wantErr  string
	}{
		{name: "nil spec", compiled: nil, wantErr: "compiled spec is nil"},
		{name: "empty digest", compiled: versionedSpec("test", "v1", ""), wantErr: "compiled spec digest is empty"},
		{name: "digest without the sha256 prefix", compiled: versionedSpec("test", "v1", strings.Repeat("ab", 32)), wantErr: "decode compiled spec digest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := storagetest.OpenTemp(t)
			err := store.SaveDeployment(t.Context(), db, "default", tt.compiled)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			if got := countRows(t, db, "spec_deployments"); got != 0 {
				t.Fatalf("a refused spec left %d deployment rows", got)
			}
		})
	}
}

func TestSaveDeploymentWithoutCanonicalJSONStoresTheCompiledSpecAsSource(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	compiled := versionedSpec("test", "v1", digestOf(1))
	compiled.CanonicalJSON = nil

	if err := store.SaveDeployment(t.Context(), db, "default", compiled); err != nil {
		t.Fatal(err)
	}

	var source, compiledIR []byte
	if err := db.QueryRowContext(t.Context(), "SELECT source_json, compiled_ir FROM spec_deployments WHERE deployment_id = ?", compiled.Digest).Scan(&source, &compiledIR); err != nil {
		t.Fatal(err)
	}
	if len(source) == 0 || !bytes.Equal(source, compiledIR) {
		t.Fatalf("source_json = %q, want the compiled spec %q", source, compiledIR)
	}
}

func TestSameDigestRedeployKeepsTheDeploymentActive(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	compiled := versionedSpec("graph-test", "1.0", digestOf(3))

	for range 2 {
		if err := store.SaveDeployment(t.Context(), db, "tenant", compiled); err != nil {
			t.Fatal(err)
		}
	}

	if got := deploymentStatus(t, db, "tenant", compiled.Digest); got != "active" {
		t.Fatalf("same-digest redeploy left the deployment %q, want active", got)
	}
	if got := countRows(t, db, "spec_deployments"); got != 1 {
		t.Fatalf("redeploy wrote %d rows, want 1", got)
	}
}

func TestNewSpecVersionRetiresThePreviousDeploymentOfTheSameTenantAndName(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	first := versionedSpec("graph-test", "1.0", digestOf(1))
	second := versionedSpec("graph-test", "2.0", digestOf(2))
	otherName := versionedSpec("other-graph", "1.0", digestOf(3))
	otherTenant := versionedSpec("graph-test", "1.0", digestOf(4))
	for tenant, compiled := range map[string]*domain.CompiledSpec{"tenant": first, "tenant-b": otherTenant} {
		if err := store.SaveDeployment(t.Context(), db, tenant, compiled); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SaveDeployment(t.Context(), db, "tenant", otherName); err != nil {
		t.Fatal(err)
	}

	if err := store.SaveDeployment(t.Context(), db, "tenant", second); err != nil {
		t.Fatal(err)
	}

	for _, want := range []struct{ tenant, digest, status string }{
		{"tenant", first.Digest, "retired"},
		{"tenant", second.Digest, "active"},
		{"tenant", otherName.Digest, "active"},
		{"tenant-b", otherTenant.Digest, "active"},
	} {
		if got := deploymentStatus(t, db, want.tenant, want.digest); got != want.status {
			t.Errorf("%s deployment %s = %q, want %q", want.tenant, want.digest, got, want.status)
		}
	}
}

func TestSaveDeploymentRegistersTheEventSchemasOfItsInputs(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	compiled := versionedSpec("graph-test", "1.0", digestOf(1))
	compiled.Inputs = []domain.Input{
		{Name: "air", SchemaRef: "bay.air_temp.observed/1.0"},
		{Name: "unlisted", SchemaRef: "not.a.builtin/1.0"},
	}

	if err := store.SaveDeployment(t.Context(), db, "tenant", compiled); err != nil {
		t.Fatal(err)
	}

	var registered []string
	rows, err := db.QueryContext(t.Context(), "SELECT schema_id FROM event_schemas ORDER BY schema_id")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		registered = append(registered, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(registered) != 1 || registered[0] != "bay.air_temp.observed/1.0" {
		t.Fatalf("registered schemas = %v, want only the built-in bay.air_temp.observed/1.0", registered)
	}
}

func TestLoadDeploymentReturnsTheSavedSpec(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	saved := versionedSpec("graph-test", "1.0", digestOf(7))
	if err := store.SaveDeployment(t.Context(), db, "tenant", saved); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.LoadDeployment(t.Context(), db, saved.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Digest != saved.Digest || loaded.Metadata.Name != saved.Metadata.Name || loaded.Metadata.Version != saved.Metadata.Version || !bytes.Equal(loaded.CanonicalJSON, saved.CanonicalJSON) {
		t.Fatalf("loaded = %+v, want %+v", loaded, saved)
	}
}

func TestLoadDeploymentReportsAnUnknownDeployment(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)

	_, err := store.LoadDeployment(t.Context(), db, digestOf(9))

	if !errors.Is(err, sql.ErrNoRows) || !strings.Contains(err.Error(), digestOf(9)) {
		t.Fatalf("err = %v, want sql.ErrNoRows naming the deployment", err)
	}
}

func TestLoadDeploymentReportsAStoredSpecItCannotDecode(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTemp(t)
	compiled := versionedSpec("graph-test", "1.0", digestOf(5))
	if err := store.SaveDeployment(t.Context(), db, "tenant", compiled); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE spec_deployments SET compiled_ir = X'7B' WHERE deployment_id = ?", compiled.Digest); err != nil {
		t.Fatal(err)
	}

	_, err := store.LoadDeployment(t.Context(), db, compiled.Digest)

	if err == nil || !strings.Contains(err.Error(), "decode deployment "+compiled.Digest) {
		t.Fatalf("err = %v, want a decode failure naming the deployment", err)
	}
}

func countRows(t *testing.T, db *storage.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
