package app

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var now = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

func ts(offset time.Duration) string { return kernel.FormatTime(now.Add(offset)) }

func within(t *testing.T, work func(ctx context.Context, tx *store.Tx, raw *sql.Tx)) {
	t.Helper()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	inTransaction(t, db, work)
}

func inTransaction(t *testing.T, db *storage.DB, work func(ctx context.Context, tx *store.Tx, raw *sql.Tx)) {
	t.Helper()
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

func queryText(t *testing.T, ctx context.Context, raw *sql.Tx, query string, args ...any) string {
	t.Helper()
	var value sql.NullString
	if err := raw.QueryRowContext(ctx, query, args...).Scan(&value); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return value.String
}

func execSQL(t *testing.T, ctx context.Context, raw *sql.Tx, query string, args ...any) {
	t.Helper()
	if _, err := raw.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func admission(id string) domain.Admission {
	digest := make([]byte, 32)
	return domain.Admission{
		EpisodeID: id, SchedulerItemID: "item-" + id, Kind: "standard", TenantID: "t", SituationID: "s-" + id, SituationVersion: 1,
		ExecutorName: "x", ExecutorVersion: "v", ModelPolicy: "p", PromptVersion: "v1",
		SnapshotSHA256: digest, PromptSHA256: digest, ObjectiveSHA256: digest, AdmissionKey: append([]byte(id), digest[len(id):]...), RequestJSON: []byte("{}"), DispatchPolicy: "shadow",
	}
}

func admit(t *testing.T, ctx context.Context, tx *store.Tx, id string) {
	t.Helper()
	must(t, Admit(ctx, tx, admission(id), now))
}

func beginAttempt(t *testing.T, ctx context.Context, tx *store.Tx, episodeID, attemptID string) domain.Identity {
	t.Helper()
	identity, err := StartAttempt(ctx, tx, episodeID, attemptID, now)
	if err != nil {
		t.Fatalf("start attempt %s: %v", attemptID, err)
	}
	return identity
}

func transition(t *testing.T, ctx context.Context, tx *store.Tx, identity domain.Identity, to domain.AttemptStatus) {
	t.Helper()
	if err := TransitionAttempt(ctx, tx, identity, to, now, nil, nil); err != nil {
		t.Fatalf("transition %s to %s: %v", identity.AttemptID, to, err)
	}
}

func attemptStatus(t *testing.T, ctx context.Context, raw *sql.Tx, attemptID string) string {
	t.Helper()
	return queryText(t, ctx, raw, "SELECT status FROM episode_attempts WHERE attempt_id = ?", attemptID)
}

func lifecycleOf(t *testing.T, ctx context.Context, raw *sql.Tx, episodeID string) string {
	t.Helper()
	return queryText(t, ctx, raw, "SELECT lifecycle_status FROM episodes WHERE episode_id = ?", episodeID)
}

func closeEpisode(t *testing.T, ctx context.Context, raw *sql.Tx, episodeID string, lifecycle domain.LifecycleStatus) {
	t.Helper()
	execSQL(t, ctx, raw, "UPDATE episodes SET lifecycle_status = ? WHERE episode_id = ?", lifecycle, episodeID)
}

type recordingSettler struct {
	episodes []string
	fail     error
}

func (s *recordingSettler) Settle(_ context.Context, _ *sql.Tx, episodeID string, _ uint64, _ string) error {
	s.episodes = append(s.episodes, episodeID)
	return s.fail
}
