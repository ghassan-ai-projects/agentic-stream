package campaign

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

type upstreamWorker struct {
	runtimev1.UnimplementedEpisodeWorkerServer
	answer workerfake.Answer
	failed string
}

func (u upstreamWorker) Execute(req *runtimev1.EpisodeRequest, stream runtimev1.EpisodeWorker_ExecuteServer) error {
	if u.failed != "" {
		return workerfake.Fail(req, stream.Send, u.failed, &runtimev1.Usage{InputTokens: 77})
	}
	return workerfake.Propose(req, stream.Send, u.answer)
}

func (u upstreamWorker) DraftPlaybookRule(req *runtimev1.DraftRequest, stream runtimev1.EpisodeWorker_DraftPlaybookRuleServer) error {
	event, err := workerfake.ProposeWhen("features.axial_mean_15m > 1.2", &runtimev1.Usage{InputTokens: 3100})(stream.Context(), req)
	if err != nil {
		return err
	}
	return stream.Send(event)
}

func terminalOf(t *testing.T, worker *Worker, req *runtimev1.EpisodeRequest) *runtimev1.Terminal {
	t.Helper()
	var terminal *runtimev1.Terminal
	if err := worker.Execute(t.Context(), req, func(event *runtimev1.EpisodeEvent) error {
		terminal = event.GetTerminal()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return terminal
}

func TestAForwardedEpisodeIsRecordedAndReplaysTheSameJudgement(t *testing.T) {
	t.Parallel()
	upstream := upstreamWorker{answer: workerfake.Answer{DiagnosisCode: "misalignment", Hypothesis: "shaft out of line", Summary: "axial rises",
		Intents: []workerfake.ProposedIntent{{Type: "schedule_maintenance", Risk: "R1", Parameters: map[string]any{"work_order": "misalignment"}}},
		Usage:   &runtimev1.Usage{InputTokens: 2222, OutputTokens: 111}}}
	path := filepath.Join(t.TempDir(), "responses.jsonl")
	forwarded := execute(t, &Worker{Recording: Record(path), Upstream: workerfake.ConnectClient(t, upstream)}, episodeRequest(wornFacts))
	replay, err := Replay(path)
	if err != nil {
		t.Fatal(err)
	}
	replayed := &Worker{Recording: replay}
	again := execute(t, replayed, episodeRequest(wornFacts))
	for _, field := range []string{"diagnosis_code", "primary_hypothesis", "summary"} {
		if forwarded[field] != again[field] {
			t.Fatalf("%s: forwarded %v, replayed %v", field, forwarded[field], again[field])
		}
	}
	intent, _ := again["intents"].([]any)[0].(map[string]any)
	if intent["type"] != "schedule_maintenance" || intent["parameters"].(map[string]any)["work_order"] != "misalignment" {
		t.Fatalf("replayed intent = %v, want the forwarded work order", intent)
	}
}

func TestAForwardedFailureReplaysAsAFailure(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "responses.jsonl")
	worker := &Worker{Recording: Record(path), Upstream: workerfake.ConnectClient(t, upstreamWorker{failed: "model_unavailable"})}
	if terminal := terminalOf(t, worker, episodeRequest(wornFacts)); terminal.GetReasonCode() != "model_unavailable" {
		t.Fatalf("forwarded terminal = %v, want the worker's failure", terminal)
	}
	replay, err := Replay(path)
	if err != nil {
		t.Fatal(err)
	}
	terminal := terminalOf(t, &Worker{Recording: replay}, episodeRequest(wornFacts))
	if terminal.GetStatus() != runtimev1.TerminalStatus_TERMINAL_STATUS_FAILED || terminal.GetUsage().GetInputTokens() != 77 {
		t.Fatalf("replayed terminal = %v, want the failure and its usage", terminal)
	}
}

func TestAForwardedDraftReplaysItsConditionAndUsage(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "responses.jsonl")
	req := &runtimev1.DraftRequest{DraftId: "d-a1", Key: &runtimev1.DraftKey{DiagnosisCode: "misalignment"},
		Cases: []*runtimev1.DraftCase{{EpisodeId: "e1"}, {EpisodeId: "e2"}}}
	forwarded, err := (&Worker{Recording: Record(path), Upstream: workerfake.ConnectClient(t, upstreamWorker{})}).Draft(t.Context(), req)
	if err != nil || forwarded.GetProposal().GetWhen() != "features.axial_mean_15m > 1.2" {
		t.Fatalf("forwarded draft = %v, %v", forwarded, err)
	}
	replay, err := Replay(path)
	if err != nil {
		t.Fatal(err)
	}
	again, err := (&Worker{Recording: replay}).Draft(context.Background(), req)
	if err != nil || again.GetProposal().GetWhen() != forwarded.GetProposal().GetWhen() || again.GetUsage().GetInputTokens() != 3100 {
		t.Fatalf("replayed draft = %v, %v; want the forwarded condition and usage", again, err)
	}
}
