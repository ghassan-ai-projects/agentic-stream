package store_test

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/control/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/kernel"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

var instant = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

var now = kernel.FormatTime(instant)

func openStore(t *testing.T) (store.Store, *storage.DB) {
	t.Helper()
	db := storagetest.OpenTemp(t)
	return store.New(db), db
}

func seedEpisode(t *testing.T, db *storage.DB, episodeID, epoch, lifecycle string) {
	t.Helper()
	sum := sha256.Sum256([]byte(episodeID))
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO episodes (
			episode_id, scheduler_item_id, tenant_id, situation_id, situation_version,
			executor_name, executor_version, model_policy, prompt_version,
			snapshot_sha256, admission_key, request_json, lifecycle_status, current_fence,
			accepted_at, policy_epoch
		) VALUES (?, ?, 'tenant', ?, 1, 'executor', 'v1', 'policy', 'prompt', ?, ?, X'7B7D', ?, 1,
			'2026-08-12T10:00:00.000000000Z', ?)`,
		episodeID, "sch-"+episodeID, "sit-"+episodeID, sum[:], sum[:], lifecycle, epoch); err != nil {
		t.Fatalf("seed episode %s: %v", episodeID, err)
	}
}
