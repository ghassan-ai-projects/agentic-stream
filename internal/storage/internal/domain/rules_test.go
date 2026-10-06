package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/migrations"
)

func TestConnectionStringSetsTheRuntimePragmas(t *testing.T) {
	t.Parallel()
	dsn := ConnectionString("/tmp/run.db")
	for _, want := range []string{"/tmp/run.db?", "foreign_keys(1)", "busy_timeout(30000)", "journal_mode(WAL)", "wal_autocheckpoint(256)", "_txlock=immediate"} {
		if !strings.Contains(dsn, want) {
			t.Errorf("dsn %q lacks %q", dsn, want)
		}
	}
}

func TestRetryBackoffDoublesUpToItsBound(t *testing.T) {
	t.Parallel()
	if RetryDelay(0) != 25*time.Millisecond || RetryDelay(1) != 50*time.Millisecond {
		t.Fatal("backoff must start at 25ms and double")
	}
	if RetryDelay(30) != time.Second {
		t.Fatalf("backoff must be bounded by one second, got %v", RetryDelay(30))
	}
	if !ShouldRetry(true, 0) || ShouldRetry(false, 0) || ShouldRetry(true, RetryAttempts-1) {
		t.Fatal("only busy failures before the last attempt retry")
	}
}

func TestPendingMigrationsKeepsOrderAndSkipsApplied(t *testing.T) {
	t.Parallel()
	all := []migrations.Migration{{Version: 1}, {Version: 2}, {Version: 3}}
	pending := PendingMigrations(all, map[int]struct{}{1: {}, 3: {}})
	if len(pending) != 1 || pending[0].Version != 2 {
		t.Fatalf("pending = %+v", pending)
	}
}

func TestReservedNamesDeriveFromTheDatabasePath(t *testing.T) {
	t.Parallel()
	if ReservationPath("/d/x.db") != "/d/x.db.replay-reservation" {
		t.Fatal("reservation path is wrong")
	}
	if got := FreshSidecars("/d/x.db"); len(got) != 2 || got[0] != "/d/x.db-wal" || got[1] != "/d/x.db-shm" {
		t.Fatalf("sidecars = %v", got)
	}
}
