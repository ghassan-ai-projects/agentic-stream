package domain

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func recordedDecision(t *testing.T, episode ReplayEpisode, entry RecordedEntry) (map[string]any, []byte, string) {
	t.Helper()
	decision := map[string]any{
		"decision_id": "dec_" + episode.EpisodeKey, "episode_id": entry.EpisodeID,
		"attempt_id": entry.AttemptID, "fence": entry.Fence, "snapshot_digest": episode.SnapshotDigest,
		"situation_id": episode.SituationID, "situation_version": episode.SituationVersion,
		"confidence": 0.9, "decision_type": "need_more_evidence", "intents": []any{},
	}
	raw, err := canonicaljson.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := canonicaljson.Digest(canonicaljson.DomainDecision, decision)
	if err != nil {
		t.Fatal(err)
	}
	return decision, raw, digest
}

func validRecordedEntry(t *testing.T, episode ReplayEpisode) RecordedEntry {
	t.Helper()
	_, raw, digest := recordedDecision(t, episode, RecordedEntry{EpisodeID: episode.EpisodeID, AttemptID: "att", Fence: 1})
	provenance, err := canonicaljson.Digest(canonicaljson.DomainOutcome, map[string]any{"episode_id": episode.EpisodeID, "attempt_id": "att", "fence": 1})
	if err != nil {
		t.Fatal(err)
	}
	return RecordedEntry{
		EpisodeKey: episode.EpisodeKey, SituationID: episode.SituationID, SituationVersion: episode.SituationVersion,
		TriggerID: episode.TriggerID, EpisodeID: episode.EpisodeID, AttemptID: "att", Fence: 1,
		AttemptProvenanceSHA256: provenance, DecisionJSON: raw, DecisionSHA256: digest,
	}
}

func TestIndexRecordedEntriesAcceptsCompleteAndRejectsDuplicates(t *testing.T) {
	t.Parallel()
	episode := ReplayEpisode{EpisodeKey: "s1/1/t1", EpisodeID: "e1", SituationID: "s1", SituationVersion: 1, TriggerID: "t1", SnapshotDigest: "sha256:" + strings.Repeat("0", 64)}
	entry := validRecordedEntry(t, episode)
	byKey, err := IndexRecordedEntries([]RecordedEntry{entry})
	if err != nil || byKey[entry.EpisodeKey].EpisodeID != "e1" {
		t.Fatalf("index = %v err=%v", byKey, err)
	}
	if _, err = IndexRecordedEntries([]RecordedEntry{entry, entry}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate accepted: %v", err)
	}
}

func TestVerifyRecordedEntryRejectsIncomplete(t *testing.T) {
	t.Parallel()
	episode := ReplayEpisode{EpisodeKey: "s1/1/t1", EpisodeID: "e1", SituationID: "s1", SituationVersion: 1, TriggerID: "t1", SnapshotDigest: "sha256:" + strings.Repeat("0", 64)}
	for name, mutate := range map[string]func(*RecordedEntry){
		"no key":        func(e *RecordedEntry) { e.EpisodeKey = "" },
		"zero fence":    func(e *RecordedEntry) { e.Fence = 0 },
		"no provenance": func(e *RecordedEntry) { e.AttemptProvenanceSHA256 = "" },
		"bad provenance": func(e *RecordedEntry) {
			e.AttemptProvenanceSHA256 = "sha256:" + strings.Repeat("f", 64)
		},
		"no decision": func(e *RecordedEntry) { e.DecisionJSON = nil },
		"bad digest":  func(e *RecordedEntry) { e.DecisionSHA256 = "sha256:" + strings.Repeat("e", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			entry := validRecordedEntry(t, episode)
			mutate(&entry)
			if err := VerifyRecordedEntry(entry); err == nil {
				t.Fatal("invalid entry accepted")
			}
		})
	}
}

func TestRequireEmptyAndExpectedRecordedKeys(t *testing.T) {
	t.Parallel()
	if err := RequireEmptyRecordedLedger(nil); err != nil {
		t.Fatalf("empty ledger rejected: %v", err)
	}
	if err := RequireEmptyRecordedLedger([]RecordedEntry{{}}); err == nil {
		t.Fatal("non-empty ledger accepted without episodes")
	}
	episodes := map[string]ReplayEpisode{"k": {}}
	if err := RequireExpectedRecordedKeys(episodes, map[string]RecordedEntry{"k": {}}); err != nil {
		t.Fatalf("expected key rejected: %v", err)
	}
	if err := RequireExpectedRecordedKeys(episodes, map[string]RecordedEntry{"other": {}}); err == nil {
		t.Fatal("unexpected ledger key accepted")
	}
}

func TestMatchRecordedMetadataUsesTheStableKey(t *testing.T) {
	t.Parallel()
	episode := ReplayEpisode{EpisodeKey: "s1/1/t1", EpisodeID: "epi-replay", SituationID: "s1", SituationVersion: 1, TriggerID: "t1"}
	entry := RecordedEntry{SituationID: "s1", SituationVersion: 1, TriggerID: "t1", EpisodeID: "epi-live-random"}
	if err := MatchRecordedMetadata(entry, episode); err != nil {
		t.Fatalf("a live episode id differing from replay's was refused: %v", err)
	}
	entry.SituationVersion = 2
	if err := MatchRecordedMetadata(entry, episode); err == nil || !strings.Contains(err.Error(), "metadata does not match") {
		t.Fatalf("mismatched metadata accepted: %v", err)
	}
}

func TestRecordedCitationAllowsALaterVersionOnly(t *testing.T) {
	t.Parallel()
	episode := ReplayEpisode{EpisodeKey: "s1/6/t1", SituationID: "s1", SituationVersion: 6, TriggerID: "t1"}
	tests := []struct {
		name     string
		decision map[string]any
		want     int
		wantErr  string
	}{
		{name: "trigger version", decision: map[string]any{"situation_id": "s1", "situation_version": float64(6)}, want: 6},
		{name: "version assembled at admission", decision: map[string]any{"situation_id": "s1", "situation_version": float64(17)}, want: 17},
		{name: "earlier version", decision: map[string]any{"situation_id": "s1", "situation_version": float64(5)}, wantErr: "mismatched situation version"},
		{name: "other situation", decision: map[string]any{"situation_id": "s2", "situation_version": float64(6)}, wantErr: "mismatched situation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := RecordedCitation(RecordedEntry{EpisodeKey: episode.EpisodeKey}, episode, tt.decision)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("cited = %d, %v; want %d", got, err, tt.want)
			}
		})
	}
}
