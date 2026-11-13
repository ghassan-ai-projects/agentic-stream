package episodeledger_test

import (
	"slices"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage/storagetest"
)

func TestOnlyASituationWhoseLatestEpisodeConcludedWithoutAnAcceptedDecisionIsUnanswered(t *testing.T) {
	t.Parallel()
	db := storagetest.OpenTempWithoutForeignKeys(t)
	for _, episode := range []struct {
		id, situation string
		version       int
		lifecycle     string
		decision      string
	}{
		{"e1", "sit-1", 3, "concluded", ""}, {"e2", "sit-1", 4, "concluded", "rejected"},
		{"e3", "sit-2", 3, "concluded", ""}, {"e4", "sit-2", 4, "concluded", "accepted"},
		{"e5", "sit-3", 3, "concluded", ""}, {"e6", "sit-3", 4, "running", ""},
	} {
		req := admission(episode.id, episode.situation)
		req.SituationVersion = episode.version
		admitEpisode(t, db, req)
		exec(t, db, "UPDATE episodes SET lifecycle_status = ? WHERE episode_id = ?", episode.lifecycle, episode.id)
		if episode.decision != "" {
			exec(t, db, `INSERT INTO decisions (decision_id, episode_id, ordinal, situation_id, situation_version, raw_json, decision_sha256,
				validation_status, validation_json, created_at) VALUES (?, ?, 1, ?, ?, X'7B7D', zeroblob(32), ?, X'7B7D', '2026-08-12T12:00:00Z')`,
				"d-"+episode.id, episode.id, episode.situation, episode.version, episode.decision)
		}
	}
	unanswered, err := episodeledger.UnansweredReasoning(t.Context(), db.DB, "tenant")
	want := []episodeledger.Unanswered{{SituationID: "sit-1", SituationVersion: 3}, {SituationID: "sit-1", SituationVersion: 4}}
	if err != nil || !slices.Equal(unanswered, want) {
		t.Fatalf("unanswered = %+v, %v; want %+v: sit-2 was answered since, sit-3 is still reasoning", unanswered, err, want)
	}
}

func exec(t *testing.T, db *storage.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}
