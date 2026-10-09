package app

import (
	"strings"
	"testing"

	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
)

func runRecorded(t *testing.T, request domain.Request, ledger domain.RecordedLedger) (domain.Result, error) {
	t.Helper()
	return RunMode(seededContext(t), domain.ModeRecorded, request, domain.Capabilities{RecordedLedger: ledger})
}

func TestRecordedPhaseValidatesACompleteLedger(t *testing.T) {
	t.Parallel()
	result, err := runRecorded(t, newRequest(t, alwaysTriggerSpec(t)), replayViewLedger{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != domain.ModeRecorded || result.CapabilityCalls == 0 || result.WorkerInvoked || result.EffectsAllowed {
		t.Fatalf("recorded result = %+v", result)
	}
}

func TestRecordedPhaseAcceptsAnEmptyLedgerWhenNothingIsExecutable(t *testing.T) {
	t.Parallel()
	result, err := runRecorded(t, newRequest(t, fixtureSpec), staticLedger{})
	if err != nil {
		t.Fatal(err)
	}
	if result.CapabilityCalls != 0 || result.WorkerInvoked || result.EffectsAllowed {
		t.Fatalf("recorded result = %+v", result)
	}
}

func TestRecordedPhaseRefusesALedgerThatDoesNotMatchTheReplay(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		spec    func(*testing.T) string
		ledger  domain.RecordedLedger
		wantErr string
	}{
		{"decision missing for an executable episode", alwaysTriggerSpec, staticLedger{}, "is missing decision"},
		{"entry for an episode the replay never produced", func(*testing.T) string { return fixtureSpec }, ledgerOfOneForeignEntry(t), "non-empty but replay produced no executable episodes"},
		{"incomplete entry", func(*testing.T) string { return fixtureSpec }, staticLedger{entries: []domain.RecordedEntry{{EpisodeKey: "situation/1/trigger", DecisionJSON: []byte(`{}`)}}}, "incomplete entry"},
		{"situation version moved", alwaysTriggerSpec, replayViewLedger{tamper: func(e *domain.RecordedEntry) { e.SituationVersion++ }}, "metadata does not match replay episode"},
		{"attempt identity changed", alwaysTriggerSpec, replayViewLedger{tamper: func(e *domain.RecordedEntry) { e.AttemptID = "att_other" }}, "attempt provenance is invalid"},
		{"duplicate episode key", alwaysTriggerSpec, duplicatingLedger{}, "duplicate episode key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runRecorded(t, newRequest(t, tc.spec(t)), tc.ledger)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("recorded replay = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}

func ledgerOfOneForeignEntry(t *testing.T) staticLedger {
	t.Helper()
	entry, err := recordedEntryFor(domain.ReplayEpisode{EpisodeKey: "situation/1/trigger", EpisodeID: "epi_foreign", SituationID: "sit_foreign", SituationVersion: 1, TriggerID: "trigger", SnapshotDigest: "sha256:" + strings.Repeat("c", 64)})
	if err != nil {
		t.Fatal(err)
	}
	return staticLedger{entries: []domain.RecordedEntry{entry}}
}
