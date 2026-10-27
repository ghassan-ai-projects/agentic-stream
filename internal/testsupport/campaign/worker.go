package campaign

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

const (
	recordedEpisode = "episode"
	recordedDraft   = "draft"
	declined        = "no condition separates these cases"
)

var draftUsage = &usage{InputTokens: 4000, OutputTokens: 600}

// Worker answers episodes and drafts like Tamoz would on the pump station.
// With an Upstream it forwards each call to that worker; without one it
// answers from canned answers over the snapshot's features. Every answer goes
// through the recording, so a campaign replays without the worker's
// judgement. With LogExchanges each forwarded call is also logged whole,
// request and every streamed event, to worker-exchanges.jsonl beside the
// recording. An abstaining worker stands for no model at all: it diagnoses
// nothing.
type Worker struct {
	Recording    *Recording
	Abstain      bool
	Upstream     runtimev1.EpisodeWorkerClient
	LogExchanges bool
}

// Server returns the worker on the episode protocol, drafting when asked to.
func (w *Worker) Server(drafts bool) *workerfake.Server {
	server := &workerfake.Server{WorkerName: "tamoz", WorkerVersion: "campaign", ExecuteFunc: w.Execute}
	if drafts {
		server.SupportedFeatures, server.DraftFunc = []string{"draft_playbook_rule"}, w.Draft
	}
	return server
}

// Execute answers one episode: forwarded, canned or replayed.
func (w *Worker) Execute(ctx context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
	entity, err := workerfake.SnapshotEntityID(req.GetSnapshotJson())
	if err != nil {
		return fmt.Errorf("episode %s: %w", req.GetEpisodeId(), err)
	}
	forwarded := false
	var given answer
	if err := w.Recording.next(recordedEpisode, entity, w.freshEpisode(ctx, req, emit, &forwarded), &given); err != nil || forwarded {
		return wrapEpisode(req, err)
	}
	return wrapEpisode(req, given.give(req, emit))
}

func (w *Worker) freshEpisode(ctx context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error, forwarded *bool) func() (any, error) {
	return func() (any, error) {
		if w.Upstream == nil {
			return w.diagnose(req.GetSnapshotJson())
		}
		*forwarded = true
		call := w.call(recordedEpisode, req)
		given, err := forwardEpisode(ctx, w.Upstream, req, call.passing(emit))
		return given, call.end(err)
	}
}

func wrapEpisode(req *runtimev1.EpisodeRequest, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("episode %s: %w", req.GetEpisodeId(), err)
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

func (a answer) give(req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
	if a.Failed != "" {
		if err := workerfake.Fail(req, emit, a.Failed, a.Usage.wire()); err != nil {
			return fmt.Errorf("give the recorded failure: %w", err)
		}
		return nil
	}
	if err := workerfake.Propose(req, emit, workerfake.Answer{Hypothesis: cmp.Or(a.Hypothesis, a.DiagnosisCode), DiagnosisCode: a.DiagnosisCode,
		Summary: cmp.Or(a.Summary, "canned pump diagnosis"), Intents: a.Intents, Usage: a.Usage.wire()}); err != nil {
		return fmt.Errorf("give the recorded answer: %w", err)
	}
	return nil
}

// Draft answers a draft: forwarded, canned or replayed. A canned draft
// proposes a rule for the keys the canned answers know a condition for and
// declines the others.
func (w *Worker) Draft(ctx context.Context, req *runtimev1.DraftRequest) (*runtimev1.DraftEvent, error) {
	var forwardedEvent *runtimev1.DraftEvent
	var given draftAnswer
	if err := w.Recording.next(recordedDraft, req.GetKey().GetDiagnosisCode(), w.freshDraft(ctx, req, &forwardedEvent), &given); err != nil {
		return nil, fmt.Errorf("draft %s: %w", req.GetDraftId(), err)
	}
	if forwardedEvent != nil {
		return forwardedEvent, nil
	}
	return given.event(ctx, req)
}

func (w *Worker) freshDraft(ctx context.Context, req *runtimev1.DraftRequest, forwarded **runtimev1.DraftEvent) func() (any, error) {
	return func() (any, error) {
		if w.Upstream == nil {
			return cannedDraft(req.GetKey().GetDiagnosisCode())
		}
		call := w.call(recordedDraft, req)
		event, err := forwardDraft(ctx, w.Upstream, req, call)
		*forwarded = event
		return draftAnswerOf(event), call.end(err)
	}
}

func cannedDraft(diagnosisCode string) (draftAnswer, error) {
	when, err := draftCondition(diagnosisCode)
	if err != nil || when == "" {
		return draftAnswer{Reason: declined}, err
	}
	return draftAnswer{When: when, Usage: draftUsage}, nil
}
