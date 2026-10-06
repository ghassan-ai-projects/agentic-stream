package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func newStore(t *testing.T) Store {
	t.Helper()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(db)
}

func TestStoreSavesSpecDeploymentAndReadsEmptyDigests(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := context.Background()
	compiled, err := spec.CompileFile(ctx, "../../../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSpecDeployment(ctx, "default", compiled); err != nil {
		t.Fatal(err)
	}
	versions, err := store.SituationVersionDigests(ctx, compiled.Digest)
	if err != nil || len(versions) != 0 {
		t.Fatalf("versions = %d err = %v", len(versions), err)
	}
	episodes, err := store.EpisodeWorklist(ctx, "default")
	if err != nil || len(episodes) != 0 {
		t.Fatalf("worklist = %d err = %v", len(episodes), err)
	}
}

func TestStoreSnapshotLookupsFailClosedOnMissingVersions(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	episode := domain.ReplayEpisode{SituationID: "missing", SituationVersion: 1}
	if _, err := store.RecordedSnapshotDigest(t.Context(), episode); err == nil || !strings.Contains(err.Error(), "load recorded snapshot digest") {
		t.Fatalf("recorded digest err = %v", err)
	}
	if _, _, err := store.ShadowSnapshot(t.Context(), episode); err == nil || !strings.Contains(err.Error(), "load shadow snapshot") {
		t.Fatalf("shadow snapshot err = %v", err)
	}
}

func TestStoreMaterializeEpisodesSucceedsWithoutPendingItems(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := context.Background()
	compiled, err := spec.CompileFile(ctx, "../../../../docs/design/examples/predictive-maintenance.situation.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MaterializeEpisodes(ctx, compiled, "default", time.Unix(0, 0).UTC()); err != nil {
		t.Fatalf("materialize on empty scheduler = %v", err)
	}
}

func bytesOf(fill byte) []byte {
	value := make([]byte, 32)
	for i := range value {
		value[i] = fill
	}
	return value
}
