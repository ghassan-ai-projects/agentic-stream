package campaign

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func loggedExchanges(t *testing.T, dir string) []exchangeShown {
	t.Helper()
	file, err := os.Open(filepath.Join(dir, exchangesFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	var logged []exchangeShown
	scanner := bufio.NewScanner(file)
	scanner.Buffer(nil, 1<<20)
	for scanner.Scan() {
		var line exchangeShown
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatal(err)
		}
		logged = append(logged, line)
	}
	return logged
}

type exchangeShown struct {
	Kind     string           `json:"kind"`
	SentAt   string           `json:"sent_at"`
	Request  map[string]any   `json:"request"`
	Response []map[string]any `json:"response"`
}

func TestAForwardedEpisodeLogsItsRequestAndEveryEventReadably(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	upstream := upstreamWorker{answer: workerfake.Answer{DiagnosisCode: "bearing_wear", Hypothesis: "worn bearing"}}
	req := episodeRequest(wornFacts)
	req.CapabilityToken = []byte("evidence-capability")
	execute(t, &Worker{Recording: Record(filepath.Join(dir, "responses.jsonl")), Upstream: workerfake.ConnectClient(t, upstream), LogExchanges: true}, req)
	logged := loggedExchanges(t, dir)
	if len(logged) != 1 || logged[0].Kind != recordedEpisode || logged[0].SentAt == "" || logged[0].Request["episode_id"] != "ep-1" {
		t.Fatalf("logged = %+v, want the one episode exchange", logged)
	}
	if _, ok := logged[0].Request["snapshot_json"].(map[string]any); !ok {
		t.Fatalf("snapshot_json = %v, want the snapshot as a JSON object", logged[0].Request["snapshot_json"])
	}
	if _, ok := logged[0].Request["capability_token"]; ok {
		t.Fatal("the log keeps the capability token")
	}
	if loggedDiagnosis(logged[0].Response) != "bearing_wear" {
		t.Fatalf("logged events = %v, want the decision document among them", logged[0].Response)
	}
}

func loggedDiagnosis(events []map[string]any) any {
	for _, event := range events {
		if decision, ok := event["decision"].(map[string]any); ok {
			document, _ := decision["decision_json"].(map[string]any)
			return document["diagnosis_code"]
		}
	}
	return nil
}

func TestAForwardedDraftLogsItsRequestAndProposal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	req := &runtimev1.DraftRequest{DraftId: "d-a1", Key: &runtimev1.DraftKey{DiagnosisCode: "misalignment"}}
	worker := &Worker{Recording: Record(filepath.Join(dir, "responses.jsonl")), Upstream: workerfake.ConnectClient(t, upstreamWorker{}), LogExchanges: true}
	if _, err := worker.Draft(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	logged := loggedExchanges(t, dir)
	proposal, _ := logged[0].Response[0]["proposal"].(map[string]any)
	if logged[0].Kind != recordedDraft || logged[0].Request["draft_id"] != "d-a1" || proposal["when"] != "features.axial_mean_15m > 1.2" {
		t.Fatalf("logged = %+v, want the draft request and its proposal", logged)
	}
}

func TestAForwardedCallIsNotLoggedUnlessAsked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	execute(t, &Worker{Recording: Record(filepath.Join(dir, "responses.jsonl")), Upstream: workerfake.ConnectClient(t, upstreamWorker{})}, episodeRequest(wornFacts))
	if _, err := os.Stat(filepath.Join(dir, exchangesFile)); !os.IsNotExist(err) {
		t.Fatalf("stat the exchange log = %v, want no log", err)
	}
}
