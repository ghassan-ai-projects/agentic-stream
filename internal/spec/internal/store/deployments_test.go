package store_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestSaveDeploymentStoresCanonicalDigestBytes(t *testing.T) {
	ctx := context.Background()
	db := storagetest.OpenTemp(t)

	digest := "sha256:" + strings.Repeat("ab", 32)
	compiled := &domain.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Metadata:      domain.Metadata{Name: "test", Version: "v1"},
		CanonicalJSON: []byte(`{"kind":"SituationSpec"}`),
		Digest:        digest,
	}
	if err := store.SaveDeployment(ctx, db, "default", compiled); err != nil {
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
	db := storagetest.OpenTemp(t)

	compiled := &domain.CompiledSpec{
		SchemaVersion: "agentic-stream/v1",
		Metadata:      domain.Metadata{Name: "test", Version: "v1"},
		CanonicalJSON: []byte(`{"kind":"SituationSpec"}`),
		Digest:        strings.Repeat("ab", 32),
	}
	if err := store.SaveDeployment(ctx, db, "default", compiled); err == nil {
		t.Fatal("expected unprefixed digest to be rejected")
	}
}
