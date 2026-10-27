package campaign

import (
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestTheCannedDiagnoserReadsTheRisingFeature(t *testing.T) {
	t.Parallel()
	for want, facts := range map[string]map[string]float64{
		"misalignment": {"axial_mean_15m": 1.3, "axial_mean_24h": 1, "vibration_mean_15m": 1.3, "vibration_mean_24h": 1},
		"bearing_wear": {"axial_mean_15m": 1, "axial_mean_24h": 1, "vibration_mean_15m": 1.3, "vibration_mean_24h": 1},
		"overload":     {"current_mean_15m": 1.3, "current_mean_24h": 1},
		"unknown":      {"vibration_mean_15m": 1.3},
	} {
		given, err := diagnose(facts)
		if err != nil || given.DiagnosisCode != want {
			t.Fatalf("diagnose(%v) = %+v (%v), want %s", facts, given, err, want)
		}
	}
}

func episodeRequest(facts string) *runtimev1.EpisodeRequest {
	return &runtimev1.EpisodeRequest{EpisodeId: "ep-1", AttemptId: "at-1", Fence: 1, TenantId: "default", SituationId: "sit-1", SituationVersion: 2,
		SnapshotSha256: make([]byte, 32), SnapshotJson: []byte(`{"entity":{"id":"site-a:pump-3"},"facts":` + facts + `}`)}
}

func execute(t *testing.T, worker *Worker, req *runtimev1.EpisodeRequest) map[string]any {
	t.Helper()
	var decision map[string]any
	err := worker.Execute(t.Context(), req, func(event *runtimev1.EpisodeEvent) error {
		if proposed := event.GetDecision(); proposed != nil {
			return json.Unmarshal(proposed.GetDecisionJson(), &decision)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return decision
}

const wornFacts = `{"vibration_mean_15m":4.2,"vibration_mean_24h":3.0,"axial_mean_15m":1.0,"axial_mean_24h":1.0}`

func TestTheWorkerBooksTheWorkOrderOfItsDiagnosisAndReplaysIt(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "responses.jsonl")
	decision := execute(t, &Worker{Recording: Record(path)}, episodeRequest(wornFacts))
	encoded, _ := json.Marshal(decision)
	if decision["diagnosis_code"] != "bearing_wear" || !strings.Contains(string(encoded), `"work_order":"bearing_wear"`) {
		t.Fatalf("decision = %s, want a bearing_wear work order", encoded)
	}
	replay, err := Replay(path)
	if err != nil {
		t.Fatal(err)
	}
	replayed := execute(t, &Worker{Recording: replay}, episodeRequest(`{}`))
	if replayed["diagnosis_code"] != "bearing_wear" {
		t.Fatalf("replayed decision = %v, want the recorded diagnosis, not a new one", replayed)
	}
	if err := (&Worker{Recording: replay}).Execute(t.Context(), episodeRequest(`{}`), nil); !errors.Is(err, errRecordingExhausted) {
		t.Fatalf("a third episode = %v, want the recording exhausted", err)
	}
}

func TestTheWorkerRefusesASnapshotWithoutEntity(t *testing.T) {
	t.Parallel()
	req := episodeRequest(`{}`)
	req.SnapshotJson = []byte(`{"facts":{}}`)
	if err := (&Worker{Recording: Record(filepath.Join(t.TempDir(), "r.jsonl"))}).Execute(t.Context(), req, nil); err == nil || !strings.Contains(err.Error(), "snapshot has no entity id") {
		t.Fatalf("Execute without entity = %v, want the missing entity", err)
	}
}

func TestTheWorkerDraftsOnlyTheKeysItKnowsAConditionFor(t *testing.T) {
	t.Parallel()
	worker := &Worker{Recording: Record(filepath.Join(t.TempDir(), "r.jsonl"))}
	for code, proposes := range map[string]bool{"bearing_wear": true, "sensor_drift": false} {
		req := &runtimev1.DraftRequest{DraftId: "rdraft_1", Key: &runtimev1.DraftKey{DiagnosisCode: code, Intent: "schedule_maintenance", Preset: code},
			Cases: []*runtimev1.DraftCase{{EpisodeId: "ep-1"}}}
		event, err := worker.Draft(t.Context(), req)
		if err != nil || (event.GetProposal() != nil) != proposes {
			t.Fatalf("draft %s = %v (%v), want a proposal %v", code, event, err, proposes)
		}
	}
	if server := worker.Server(true); server.DraftFunc == nil || len(server.SupportedFeatures) != 1 {
		t.Fatalf("drafting server = %+v", server)
	}
}

func TestAReplayRefusesABrokenRecording(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "broken.jsonl", "{\n")
	if _, err := Replay(filepath.Join(dir, "broken.jsonl")); err == nil || !strings.Contains(err.Error(), "decode recording line 1") {
		t.Fatalf("Replay of a broken recording = %v, want its first line refused", err)
	}
	if _, err := Replay(filepath.Join(dir, "missing.jsonl")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Replay of a missing recording = %v, want not exist", err)
	}
}

func TestAnAbstainingWorkerDiagnosesNothing(t *testing.T) {
	t.Parallel()
	decision := execute(t, &Worker{Recording: Record(filepath.Join(t.TempDir(), "r.jsonl")), Abstain: true}, episodeRequest(wornFacts))
	if decision["diagnosis_code"] != "unknown" || len(decision["intents"].([]any)) != 0 {
		t.Fatalf("decision = %v, want no diagnosis and no intent", decision)
	}
}
