package app_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"

	store "github.com/ghassan-ai-projects/agentic-stream/internal/episodes/internal/store"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
)

func TestRunnerOrdersEpisodesByChronologicalAcceptedAt(t *testing.T) {
	ctx := context.Background()
	db, err := storagetest.Open(ctx, filepath.Join(t.TempDir(), "accepted-order.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	seedEpisode(t, ctx, db, "epi-order-earlier")
	if _, err := db.ExecContext(ctx, "UPDATE episodes SET accepted_at = '2026-08-12T10:00:00.000000000Z' WHERE episode_id = 'epi-order-earlier'"); err != nil {
		t.Fatalf("normalize earlier accepted_at: %v", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys for second fixture: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, admission_key, request_json, lifecycle_status, current_fence, accepted_at
		) VALUES ('epi-order-later', 'sch-order-later', 'tenant', 'sit-order-later', 1,
			'executor', 'v1', 'policy', 'prompt', zeroblob(32), X'0101010101010101010101010101010101010101010101010101010101010101', X'7B7D',
			'admitted', 0, '2026-08-12T10:00:00.500000000Z')`); err != nil {
		t.Fatalf("seed later episode: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type,
			entity_id, partition_id, occurrence_id, current_version, last_reasoned_version,
			phase, status, first_event_time, latest_event_time, updated_at, created_at
		) VALUES ('sit-order-later', 'tenant', 'dep-order', 'test', 'thing', 'ent-order-later', 0,
			'occ-order-later', 1, 0, 'candidate', 'open', '2026-08-12T10:00:00Z',
			'2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z')`); err != nil {
		t.Fatalf("seed later situation: %v", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("restore foreign keys for second fixture: %v", err)
	}

	executor := &recordingDeclinedExecutor{}
	runner := app.NewRunner(store.New(db), executor, sources.Physical(), sources.Deterministic())
	processed, err := runner.RunOnce(ctx, "tenant")
	if err != nil || !processed {
		t.Fatalf("run processed=%v err=%v", processed, err)
	}
	if len(executor.episodeIDs) != 1 || executor.episodeIDs[0] != "epi-order-earlier" {
		t.Fatalf("first dispatched episodes=%v, want [epi-order-earlier]", executor.episodeIDs)
	}
}

type recordingDeclinedExecutor struct {
	episodeIDs []string
}

func (e *recordingDeclinedExecutor) Execute(_ context.Context, req *app.Request) (*app.Outcome, error) {
	e.episodeIDs = append(e.episodeIDs, req.EpisodeID)
	return &app.Outcome{Status: string(episodeledger.AttemptDeclined), AttemptID: req.AttemptID, Fence: req.Fence}, nil
}

var _ app.Executor = (*recordingDeclinedExecutor)(nil)
