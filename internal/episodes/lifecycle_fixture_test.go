package episodes

import (
	"context"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

func seedEpisode(t *testing.T, ctx context.Context, db *storage.DB, episodeID string) {
	t.Helper()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	digest := make([]byte, 32)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, admission_key, request_json, lifecycle_status, current_fence, accepted_at
		) VALUES (?, 'sch-test', 'tenant', 'sit-test', 1, 'executor', 'v1', 'policy', 'prompt', ?, ?, X'7B7D', 'admitted', 0, ?)`,
		episodeID, digest, digest, "2026-08-12T10:00:00Z"); err != nil {
		t.Fatalf("seed episode: %v", err)
	}
	// P8 (freshness): the dispatch-time situation-version recheck reads the
	// live situations registry — seed the row the episode is bound to.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO situations (
			situation_id, tenant_id, deployment_id, situation_type, entity_type,
			entity_id, partition_id, occurrence_id, current_version,
			last_reasoned_version, phase, status, first_event_time, latest_event_time, updated_at, created_at
		) VALUES ('sit-test', 'tenant', 'dep-test', 'test', 'thing', 'ent-1', 0, 'occ-test', 1, 0, 'candidate', 'open',
			'2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z')`); err != nil {
		t.Fatalf("seed situation registry: %v", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
}
