package store_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var at = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func within(t *testing.T, work func(ctx context.Context, tx *store.Tx, raw *sql.Tx)) {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)

	if err := db.WithTx(t.Context(), func(raw *sql.Tx) error {
		work(t.Context(), store.Join(raw), raw)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func execSQL(t *testing.T, ctx context.Context, raw *sql.Tx, query string, args ...any) {
	t.Helper()
	if _, err := raw.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func queryText(t *testing.T, ctx context.Context, raw *sql.Tx, query string, args ...any) string {
	t.Helper()
	var value sql.NullString
	if err := raw.QueryRowContext(ctx, query, args...).Scan(&value); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return value.String
}

func ts(offset time.Duration) string { return kernel.FormatTime(at.Add(offset)) }

func make32(seed byte) []byte {
	key := make([]byte, 32)
	key[0] = seed
	return key
}

func item(id, trigger string) domain.SchedulerItem {
	return domain.SchedulerItem{SchedulerItemID: id, TriggerID: trigger, SituationID: "s", SituationVersion: 1, Kind: "standard", Lane: "fast", Status: "pending", ExpiresAt: at.Add(time.Hour)}
}

func admitted(id string) domain.Admission {
	digest := make([]byte, 32)
	return domain.Admission{
		EpisodeID: id, SchedulerItemID: "item-" + id, Kind: "standard", TenantID: "t", SituationID: "s-" + id, SituationVersion: 1,
		ExecutorName: "x", ExecutorVersion: "v", ModelPolicy: "p", PromptVersion: "v1", DispatchPolicy: "shadow",
		SnapshotSHA256: digest, PromptSHA256: digest, ObjectiveSHA256: digest, AdmissionKey: append([]byte(id), digest[len(id):]...), RequestJSON: []byte("{}"),
	}
}

func admit(t *testing.T, ctx context.Context, tx *store.Tx, id string) {
	t.Helper()
	must(t, tx.InsertEpisode(ctx, admitted(id), at))
}

func startAttempt(t *testing.T, ctx context.Context, tx *store.Tx, episodeID, attemptID string, fence int64) domain.Identity {
	t.Helper()
	identity := domain.Identity{EpisodeID: episodeID, AttemptID: attemptID, Fence: fence}
	must(t, tx.InsertAttempt(ctx, identity, at))
	must(t, tx.RecordEpisodeAttempt(ctx, identity, at))
	return identity
}

func TestAHandleWithoutADatabaseReportsItselfClosed(t *testing.T) {
	t.Parallel()
	if store.Join(nil).Open() || store.Reader(nil).Open() {
		t.Fatal("nil handles reported open")
	}
	db := storagetest.OpenTemp(t)
	if !store.Reader(db.DB).Open() {
		t.Fatal("a reader over a database reported closed")
	}
}
