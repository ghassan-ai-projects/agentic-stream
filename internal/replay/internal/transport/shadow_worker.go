package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/grpc"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodeledger"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/remote"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

// shadowRuntimeInstance names replay to the worker during the handshake.
const shadowRuntimeInstance = "agentic-stream-replay-shadow"

// ShadowWorker is the shadow candidate behind the EpisodeWorker protocol. It
// sends the episode request replay assembled, offers no evidence tools (the
// snapshot is the whole input) and never retries: one request per trial.
type ShadowWorker struct {
	conn     *grpc.ClientConn
	executor *remote.Executor
	name     string
}

var _ domain.ShadowExecutor = (*ShadowWorker)(nil)

// DialShadowWorker connects to a worker on its Unix socket.
func DialShadowWorker(ctx context.Context, socketPath, name string) (*ShadowWorker, error) {
	conn, err := worker.DialEpisodeWorkerSocketTLS(ctx, socketPath, nil)
	if err != nil {
		return nil, fmt.Errorf("dial shadow worker: %w", err)
	}
	executor := remote.NewExecutor(runtimev1.NewEpisodeWorkerClient(conn), name, shadowRuntimeInstance, nil)
	return &ShadowWorker{conn: conn, executor: executor, name: name}, nil
}

// Close releases the worker connection.
func (w *ShadowWorker) Close() error {
	return w.conn.Close() //nolint:wrapcheck // Raw close error; callers discard it on teardown.
}

// ExecuteShadow runs one trial on the worker and returns its decision.
func (w *ShadowWorker) ExecuteShadow(ctx context.Context, input domain.ShadowInput) (domain.ShadowOutput, error) {
	request, err := shadowEpisodeRequest(input)
	if err != nil {
		return domain.ShadowOutput{}, err
	}
	outcome, err := w.executor.Execute(ctx, request)
	if err != nil {
		return domain.ShadowOutput{}, fmt.Errorf("worker %s: %w", w.name, err)
	}
	return w.shadowOutput(input, outcome)
}

// shadowEpisodeRequest is the replayed episode as a worker attempt: the
// synthetic attempt and fence of the trial, dispatched in shadow mode.
func shadowEpisodeRequest(input domain.ShadowInput) (*episodes.Request, error) {
	prompt, err := promptProvenance(input.Request.RequestJSON)
	if err != nil {
		return nil, err
	}
	return &episodes.Request{
		EpisodeID: input.EpisodeID, TenantID: input.TenantID, SituationID: input.SituationID, SituationVersion: input.SituationVersion,
		ExecutorName: input.Request.ExecutorName, ExecutorVersion: input.Request.ExecutorVersion, ModelPolicy: input.Request.ModelPolicy,
		PromptVersion: input.Request.PromptVersion, PromptSHA256: prompt.PromptSHA256, ObjectiveSHA256: prompt.ObjectiveSHA256,
		SnapshotSHA256: input.Request.SnapshotSHA256, AttemptID: input.AttemptID, Fence: input.Fence,
		RequestJSON: input.Request.RequestJSON, DispatchPolicy: "shadow",
	}, nil
}

type requestPrompt struct {
	PromptSHA256    string `json:"prompt_sha256"`
	ObjectiveSHA256 string `json:"objective_sha256"`
}

func promptProvenance(requestJSON []byte) (requestPrompt, error) {
	var request struct {
		Executor requestPrompt `json:"executor"`
	}
	if err := json.Unmarshal(requestJSON, &request); err != nil {
		return requestPrompt{}, fmt.Errorf("decode shadow episode request: %w", err)
	}
	return request.Executor, nil
}

// shadowOutput accepts only a produced decision, in canonical bytes, with a
// manifest that digests what the runtime asked the worker to run.
func (w *ShadowWorker) shadowOutput(input domain.ShadowInput, outcome *episodes.Outcome) (domain.ShadowOutput, error) {
	if outcome.Status != string(episodeledger.AttemptProduced) {
		return domain.ShadowOutput{}, fmt.Errorf("worker %s produced no decision: %s (%s)", w.name, outcome.Status, strings.Join(outcome.Reasons, ", "))
	}
	decision, err := canonicaljson.Marshal(json.RawMessage(outcome.DecisionJSON))
	if err != nil {
		return domain.ShadowOutput{}, fmt.Errorf("canonicalize worker decision: %w", err)
	}
	manifest, err := candidateManifest(input, w.name)
	if err != nil {
		return domain.ShadowOutput{}, err
	}
	return domain.ShadowOutput{ExecutorVersion: w.name + "@" + input.Request.ExecutorVersion, ManifestSHA256: manifest, DecisionJSON: decision, DecisionSHA256: outcome.DecisionSHA256}, nil
}

func candidateManifest(input domain.ShadowInput, name string) (string, error) {
	prompt, err := promptProvenance(input.Request.RequestJSON)
	if err != nil {
		return "", err
	}
	manifest, err := canonicaljson.Digest(canonicaljson.DomainShadowComparison, map[string]any{
		"worker": name, "executor_name": input.Request.ExecutorName, "executor_version": input.Request.ExecutorVersion,
		"model_policy": input.Request.ModelPolicy, "prompt_version": input.Request.PromptVersion,
		"prompt_sha256": prompt.PromptSHA256, "objective_sha256": prompt.ObjectiveSHA256,
	})
	if err != nil {
		return "", fmt.Errorf("digest candidate manifest: %w", err)
	}
	return manifest, nil
}
