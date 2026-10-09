package transport

import (
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc"

	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/replay/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func shadowTrialInput() domain.ShadowInput {
	return domain.ShadowInput{
		TenantID: "default", EpisodeID: "epi_1", SituationID: "sit_1", SituationVersion: 6, AttemptID: "shadow-attempt/epi_1", Fence: 1,
		Request: domain.EpisodeRequest{
			ExecutorName: "tamoz", ExecutorVersion: "sha256:" + strings.Repeat("a", 64), ModelPolicy: "policy", PromptVersion: "v1",
			SnapshotSHA256: "sha256:" + strings.Repeat("b", 64),
			RequestJSON:    []byte(`{"executor":{"prompt_sha256":"sha256:p","objective_sha256":"sha256:o"}}`),
		},
	}
}

func TestShadowEpisodeRequestIsAShadowAttempt(t *testing.T) {
	t.Parallel()
	request, err := shadowEpisodeRequest(shadowTrialInput())
	if err != nil {
		t.Fatal(err)
	}
	if request.DispatchPolicy != "shadow" || request.AttemptID != "shadow-attempt/epi_1" || request.Fence != 1 || request.PromptSHA256 != "sha256:p" || request.ObjectiveSHA256 != "sha256:o" || request.SituationVersion != 6 {
		t.Fatalf("shadow request = %+v", request)
	}
	broken := shadowTrialInput()
	broken.Request.RequestJSON = []byte("{")
	if _, err := shadowEpisodeRequest(broken); err == nil || !strings.Contains(err.Error(), "decode shadow episode request") {
		t.Fatalf("a malformed request was accepted: %v", err)
	}
}

func TestShadowOutputAcceptsOnlyAProducedDecision(t *testing.T) {
	t.Parallel()
	shadow := &ShadowWorker{name: "tamoz"}
	if _, err := shadow.shadowOutput(shadowTrialInput(), &episodes.Outcome{Status: "declined", Reasons: []string{"no_evidence"}}); err == nil || !strings.Contains(err.Error(), "produced no decision: declined (no_evidence)") {
		t.Fatalf("a declined attempt was accepted: %v", err)
	}
	output, err := shadow.shadowOutput(shadowTrialInput(), &episodes.Outcome{Status: "produced", DecisionJSON: []byte(`{"b": 1, "a": 2}`), DecisionSHA256: "sha256:d"})
	if err != nil {
		t.Fatal(err)
	}
	if string(output.DecisionJSON) != `{"a":2,"b":1}` || output.ManifestSHA256 == "" || output.ExecutorVersion != "tamoz@sha256:"+strings.Repeat("a", 64) {
		t.Fatalf("shadow output = %+v", output)
	}
}

func TestDialShadowWorkerRefusesAnythingButAnAbsoluteSocketPath(t *testing.T) {
	t.Parallel()
	if _, err := DialShadowWorker(t.Context(), "worker.sock", "tamoz"); err == nil || !strings.Contains(err.Error(), "dial shadow worker") {
		t.Fatalf("a relative socket path = %v, want dial shadow worker failure", err)
	}
}

func TestShadowWorkerNamesItselfInTheErrorOfAFailedExecution(t *testing.T) {
	t.Parallel()
	socketPath := filepath.Join(workerfake.SocketDir(t), "worker.sock")
	listener, err := worker.ListenEvidenceSocket(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	runtimev1.RegisterEpisodeWorkerServer(server, &workerfake.Server{WorkerName: "tamoz", WorkerVersion: "test"})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	shadow, err := DialShadowWorker(t.Context(), socketPath, "tamoz")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := shadow.ExecuteShadow(t.Context(), shadowTrialInput()); err == nil || !strings.Contains(err.Error(), "worker tamoz:") {
		t.Fatalf("an unservable request = %v, want an error naming worker tamoz", err)
	}
	if err := shadow.Close(); err != nil {
		t.Fatalf("close the shadow worker connection: %v", err)
	}
}
