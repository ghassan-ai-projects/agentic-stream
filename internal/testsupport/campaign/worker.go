package campaign

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

const (
	recordedEpisode = "episode"
	recordedDraft   = "draft"
)

var draftUsage = &runtimev1.Usage{InputTokens: 4000, OutputTokens: 600}

// Worker answers episodes and drafts like Tamoz would on the pump station,
// from canned answers over the snapshot's features. Every answer goes through
// the recording, so a campaign replays without the worker's judgement. An
// abstaining worker stands for no model at all: it diagnoses nothing.
type Worker struct {
	Recording *Recording
	Abstain   bool
}

// Server returns the worker on the episode protocol, drafting when asked to.
func (w *Worker) Server(drafts bool) *workerfake.Server {
	server := &workerfake.Server{WorkerName: "tamoz", WorkerVersion: "campaign-canned", ExecuteFunc: w.Execute}
	if drafts {
		server.SupportedFeatures, server.DraftFunc = []string{"draft_playbook_rule"}, w.Draft
	}
	return server
}

// Execute answers one episode with a diagnosis and at most one intent.
func (w *Worker) Execute(_ context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
	entity, err := workerfake.SnapshotEntityID(req.GetSnapshotJson())
	if err != nil {
		return fmt.Errorf("episode %s: %w", req.GetEpisodeId(), err)
	}
	var given answer
	if err := w.Recording.next(recordedEpisode, entity, func() (any, error) { return w.diagnose(req.GetSnapshotJson()) }, &given); err != nil {
		return fmt.Errorf("episode %s: %w", req.GetEpisodeId(), err)
	}
	if err := workerfake.Propose(req, emit, given.workerAnswer()); err != nil {
		return fmt.Errorf("episode %s: %w", req.GetEpisodeId(), err)
	}
	return nil
}

func (w *Worker) diagnose(snapshotJSON []byte) (answer, error) {
	if w.Abstain {
		return unknown, nil
	}
	return diagnoseSnapshot(snapshotJSON)
}

func diagnoseSnapshot(snapshotJSON []byte) (answer, error) {
	var snapshot struct {
		Facts map[string]any `json:"facts"`
	}
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return answer{}, fmt.Errorf("decode the snapshot facts: %w", err)
	}
	facts := map[string]float64{}
	for name, value := range snapshot.Facts {
		if number, ok := value.(float64); ok {
			facts[name] = number
		}
	}
	return diagnose(facts)
}

func (a answer) workerAnswer() workerfake.Answer {
	return workerfake.Answer{Hypothesis: a.DiagnosisCode, DiagnosisCode: a.DiagnosisCode, Summary: "canned pump diagnosis", Intent: a.Intent}
}

// Draft proposes a rule for the keys the canned answers know a condition for
// and declines the others.
func (w *Worker) Draft(ctx context.Context, req *runtimev1.DraftRequest) (*runtimev1.DraftEvent, error) {
	code := req.GetKey().GetDiagnosisCode()
	var when string
	if err := w.Recording.next(recordedDraft, code, func() (any, error) { return draftCondition(code) }, &when); err != nil {
		return nil, fmt.Errorf("draft %s: %w", req.GetDraftId(), err)
	}
	if when == "" {
		return workerfake.DeclineDraft("no condition separates these cases")(ctx, req)
	}
	return workerfake.ProposeWhen(when, draftUsage)(ctx, req)
}
