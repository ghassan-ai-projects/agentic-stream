package spec_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestSaveDeploymentStoresCanonicalDigestBytes(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	digest := "sha256:" + strings.Repeat("ab", 32)
	compiled := &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Metadata:      spec.Metadata{Name: "test", Version: "v1"},
		CanonicalJSON: []byte(`{"kind":"SituationSpec"}`),
		Digest:        digest,
	}
	if err := spec.SaveDeployment(ctx, db, "default", compiled); err != nil {
		t.Fatalf("save deployment: %v", err)
	}

	var got []byte
	if err := db.QueryRowContext(ctx,
		"SELECT spec_sha256 FROM spec_deployments WHERE deployment_id = ?", digest,
	).Scan(&got); err != nil {
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

func TestSaveDeploymentRejectsUnprefixedDigest(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	compiled := &spec.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Metadata:      spec.Metadata{Name: "test", Version: "v1"},
		CanonicalJSON: []byte(`{"kind":"SituationSpec"}`),
		Digest:        strings.Repeat("ab", 32),
	}
	if err := spec.SaveDeployment(ctx, db, "default", compiled); err == nil {
		t.Fatal("expected unprefixed digest to be rejected")
	}
}
