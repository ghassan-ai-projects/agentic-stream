package domain

import (
	"strings"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
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
	for name, tc := range map[string]struct {
		mutate  func(*RecordedEntry)
		wantErr string
	}{
		"no key":        {func(e *RecordedEntry) { e.EpisodeKey = "" }, "incomplete entry"},
		"zero fence":    {func(e *RecordedEntry) { e.Fence = 0 }, "incomplete entry"},
		"no provenance": {func(e *RecordedEntry) { e.AttemptProvenanceSHA256 = "" }, "incomplete entry"},
		"no decision":   {func(e *RecordedEntry) { e.DecisionJSON = nil }, "incomplete entry"},
		"bad provenance": {func(e *RecordedEntry) {
			e.AttemptProvenanceSHA256 = "sha256:" + strings.Repeat("f", 64)
		}, "attempt provenance is invalid"},
		"ambiguous decision bytes": {func(e *RecordedEntry) {
			e.DecisionJSON = contractstest.AmbiguousKeyJSON(e.DecisionJSON, "decision_id")
		}, `recorded ledger decision "s1/1/t1"`},
		"bad digest": {func(e *RecordedEntry) { e.DecisionSHA256 = "sha256:" + strings.Repeat("e", 64) }, `recorded ledger decision "s1/1/t1"`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			entry := validRecordedEntry(t, episode)
			tc.mutate(&entry)
			if err := VerifyRecordedEntry(entry); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("VerifyRecordedEntry = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestRequireEmptyAndExpectedRecordedKeys(t *testing.T) {
	t.Parallel()
	if err := RequireEmptyRecordedLedger(nil); err != nil {
		t.Fatalf("empty ledger rejected: %v", err)
	}
	if err := RequireEmptyRecordedLedger([]RecordedEntry{{}}); err == nil || !strings.Contains(err.Error(), "non-empty but replay produced no executable episodes") {
		t.Fatalf("a non-empty ledger without episodes = %v, want non-empty but replay produced no executable episodes", err)
	}
	episodes := map[string]ReplayEpisode{"k": {}}
	if err := RequireExpectedRecordedKeys(episodes, map[string]RecordedEntry{"k": {}}); err != nil {
		t.Fatalf("expected key rejected: %v", err)
	}
	if err := RequireExpectedRecordedKeys(episodes, map[string]RecordedEntry{"other": {}}); err == nil || !strings.Contains(err.Error(), `unexpected decision "other"`) {
		t.Fatalf("an unexpected ledger key = %v, want unexpected decision \"other\"", err)
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

func TestValidateRecordedAttemptRejectsEachIdentityMismatch(t *testing.T) {
	t.Parallel()
	entry := RecordedEntry{EpisodeKey: "episode", EpisodeID: "ep", AttemptID: "attempt", Fence: 2}
	good := func() map[string]any {
		return map[string]any{"episode_id": "ep", "attempt_id": "attempt", "fence": float64(2)}
	}
	if err := ValidateRecordedAttempt(entry, good()); err != nil {
		t.Fatalf("matching identity rejected: %v", err)
	}
	for name, mutate := range map[string]struct {
		change  func(map[string]any)
		wantErr string
	}{
		"episode": {func(d map[string]any) { d["episode_id"] = "other" }, "mismatched episode identity"},
		"attempt": {func(d map[string]any) { d["attempt_id"] = "other" }, "mismatched attempt identity"},
		"fence":   {func(d map[string]any) { d["fence"] = float64(3) }, "mismatched fence"},
		"missing": {func(d map[string]any) { delete(d, "fence") }, "mismatched fence"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			decision := good()
			mutate.change(decision)
			if err := ValidateRecordedAttempt(entry, decision); err == nil || !strings.Contains(err.Error(), mutate.wantErr) {
				t.Fatalf("ValidateRecordedAttempt = %v, want an error containing %q", err, mutate.wantErr)
			}
		})
	}
}

func TestDecodeRecordedDecisionRequiresCanonicalJSON(t *testing.T) {
	t.Parallel()
	entry := RecordedEntry{EpisodeKey: "episode", DecisionJSON: []byte(`{"a":1,"b":2}`)}
	decision, err := DecodeRecordedDecision(entry)
	if err != nil || decision["b"] != float64(2) {
		t.Fatalf("canonical decision = %v, %v", decision, err)
	}
	entry.DecisionJSON = []byte(`{"b": 2, "a": 1}`)
	if _, err := DecodeRecordedDecision(entry); err == nil || !strings.Contains(err.Error(), "is not canonical JSON") {
		t.Fatalf("a non-canonical decision = %v, want is not canonical JSON", err)
	}
	entry.DecisionJSON = []byte(`{`)
	if _, err := DecodeRecordedDecision(entry); err == nil || !strings.Contains(err.Error(), "canonicalize recorded decision") {
		t.Fatalf("a malformed decision = %v, want canonicalize recorded decision", err)
	}
}

func TestValidateRecordedSnapshotRequiresTheReplayedSnapshotDigest(t *testing.T) {
	t.Parallel()
	replayed := []byte(strings.Repeat("\x07", 32))
	entry := RecordedEntry{EpisodeKey: "episode"}
	if err := ValidateRecordedSnapshot(entry, map[string]any{"snapshot_digest": canonicaljson.EncodeDigest(replayed)}, replayed); err != nil {
		t.Fatalf("matching snapshot digest rejected: %v", err)
	}
	if err := ValidateRecordedSnapshot(entry, map[string]any{"snapshot_digest": "sha256:" + strings.Repeat("0", 64)}, replayed); err == nil || !strings.Contains(err.Error(), "mismatched snapshot digest") {
		t.Fatalf("a foreign snapshot digest = %v, want mismatched snapshot digest", err)
	}
}
