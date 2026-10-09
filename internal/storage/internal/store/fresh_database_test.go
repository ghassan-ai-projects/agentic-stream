package store_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func TestOpenFreshReservesPathUntilClose(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "replay.db")

	db, err := storage.OpenFresh(ctx, path)
	if err != nil {
		t.Fatalf("open fresh: %v", err)
	}
	if opened, err := storage.Open(ctx, path); err == nil {
		_ = opened.Close()
		t.Fatal("Open succeeded on a path reserved by an active replay")
	} else if !strings.Contains(err.Error(), "reserved by an active replay") {
		t.Fatalf("Open on a reserved path = %v, want the reservation named", err)
	}
	if again, err := storage.OpenFresh(ctx, path); err == nil {
		_ = again.Close()
		t.Fatal("a second fresh open reused a reserved path")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(path + ".replay-reservation"); !os.IsNotExist(err) {
		t.Fatalf("reservation not released: %v", err)
	}
	if again, err := storage.OpenFresh(ctx, path); err == nil {
		_ = again.Close()
		t.Fatal("OpenFresh accepted an existing database file")
	}
	if _, err := os.Stat(path + ".replay-reservation"); !os.IsNotExist(err) {
		t.Fatalf("failed fresh open leaked its reservation: %v", err)
	}
}

func TestOpenFreshRejectsCollisionsWithoutRemovingExistingFiles(t *testing.T) {
	t.Parallel()
	for _, suffix := range []string{"", "-wal", "-shm", ".replay-reservation"} {
		t.Run(suffix, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "replay.db")
			collision := path + suffix
			original := []byte("existing owner data")
			if err := os.WriteFile(collision, original, 0o600); err != nil {
				t.Fatal(err)
			}
			if db, err := storage.OpenFresh(t.Context(), path); err == nil {
				_ = db.Close()
				t.Fatal("fresh open accepted a collision")
			}
			data, err := os.ReadFile(collision)
			if err != nil || string(data) != string(original) {
				t.Fatalf("collision changed: data=%q err=%v", data, err)
			}
			if suffix != ".replay-reservation" {
				if _, err := os.Lstat(path + ".replay-reservation"); !os.IsNotExist(err) {
					t.Fatalf("failed reservation leaked: %v", err)
				}
			}
		})
	}
}

func TestOpenFreshRejectsDanglingSidecarBeforeDatabaseCollision(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "replay.db")
	if err := os.WriteFile(path, []byte("existing database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path+".missing", path+"-wal"); err != nil {
		t.Fatal(err)
	}
	_, err := storage.OpenFresh(t.Context(), path)
	if err == nil || !strings.Contains(err.Error(), "fresh database sidecar already exists") {
		t.Fatalf("sidecar must precede database collision: %v", err)
	}
	if _, err := os.Lstat(path + "-wal"); err != nil {
		t.Fatalf("existing sidecar was removed: %v", err)
	}
	if _, err := os.Lstat(path + ".replay-reservation"); !os.IsNotExist(err) {
		t.Fatalf("failed open leaked its reservation: %v", err)
	}
}

func TestOpenFreshReleasesReservationWhenMigrationIsCanceled(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "replay.db")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := storage.OpenFresh(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled open = %v", err)
	}
	if _, err := os.Lstat(path + ".replay-reservation"); !os.IsNotExist(err) {
		t.Fatalf("migration failure leaked its reservation: %v", err)
	}
	// The reserved database file survives failure; only our reservation is owned.
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("reserved database file was removed: %v", err)
	}
}
