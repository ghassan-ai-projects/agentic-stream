package store

import (
	"strings"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
)

func TestSourceLedgerReadsAcceptedDecisionsOfTheDeployedSpec(t *testing.T) {
	t.Parallel()
	store, digest := replayedStore(t)
	episodes, err := store.EpisodeWorklist(t.Context(), "default")
	if err != nil || len(episodes) == 0 {
		t.Fatalf("worklist = %d, %v", len(episodes), err)
	}
	first := episodes[0]
	if _, err := store.DB.ExecContext(t.Context(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(t.Context(), `INSERT INTO decisions (decision_id, episode_id, attempt_id, fence, ordinal, situation_id, situation_version,
		raw_json, decision_sha256, validation_status, validation_json, created_at)
		VALUES ('dec-1', ?, 'att-1', 1, 1, ?, ?, CAST('{}' AS BLOB), zeroblob(32), 'accepted', CAST('{}' AS BLOB), '2026-10-08T00:00:00Z')`,
		first.EpisodeID, first.SituationID, first.SituationVersion); err != nil {
		t.Fatal(err)
	}
	ledger := NewSourceLedger(store.DB.DB, "default", digest)
	if err := ledger.RequireDeployment(t.Context()); err != nil {
		t.Fatalf("deployed spec refused: %v", err)
	}
	if err := NewSourceLedger(store.DB.DB, "default", "sha256:other").RequireDeployment(t.Context()); err == nil || !strings.Contains(err.Error(), "never deployed") {
		t.Fatalf("a foreign spec was accepted: %v", err)
	}
	entries, err := ledger.Entries(t.Context())
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %+v, %v", entries, err)
	}
	entry := entries[0]
	provenance, _ := domain.AttemptProvenance(entry)
	if entry.EpisodeKey != first.EpisodeKey || entry.AttemptID != "att-1" || entry.Fence != 1 || entry.AttemptProvenanceSHA256 != provenance || !strings.HasPrefix(entry.DecisionSHA256, "sha256:") {
		t.Fatalf("entry = %+v", entry)
	}
	request, err := store.ShadowRequest(t.Context(), first)
	if err != nil || len(request.RequestJSON) == 0 || request.ExecutorName == "" || !strings.HasPrefix(request.SnapshotSHA256, "sha256:") {
		t.Fatalf("shadow request = %+v, %v", request, err)
	}
}
