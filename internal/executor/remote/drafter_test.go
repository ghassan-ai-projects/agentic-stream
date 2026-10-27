package remote_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/draftport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/remote"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func draftRequest() draftport.Request {
	return draftport.Request{DraftID: "rdraft_1", TenantID: "t",
		Key:          draftport.Key{DiagnosisCode: "wear", KindDigest: "sha256:k", Intent: "service", Preset: "wear"},
		Cases:        []draftport.Case{{EpisodeID: "e1", Verdict: "verified", Features: map[string]any{"level": 4.2}}, {EpisodeID: "e2", Verdict: "verified"}},
		CurrentRules: []draftport.Rule{{RuleID: "r", Version: 1, When: "features.level > 1.0", Mode: "shadow"}},
		Budget:       draftport.Budget{WallTime: time.Minute, ModelCalls: 2}}
}

func TestTheDrafterReturnsTheWorkersProposalAndUsage(t *testing.T) {
	t.Parallel()
	server := &workerfake.Server{WorkerName: "worker-1", WorkerVersion: "test", SupportedFeatures: []string{draftport.Feature},
		DraftFunc: workerfake.ProposeWhen("features.level > 4.0", &runtimev1.Usage{InputTokens: 1200, OutputTokens: 80, CostMicrounits: 9})}
	reply, err := remote.NewDrafter(workerfake.ConnectClient(t, server), "worker-1", "epoch").Draft(t.Context(), draftRequest())
	if err != nil || reply.Proposal == nil || reply.Proposal.When != "features.level > 4.0" || len(reply.Proposal.CitedEpisodeIDs) != 2 ||
		reply.Usage.InputTokens != 1200 || reply.Usage.CostMicrounits != 9 {
		t.Fatalf("reply = %+v, %v; want the canned proposal citing both cases", reply, err)
	}
}

func TestTheDrafterReportsAWorkerThatDeclinesOrCannotDraft(t *testing.T) {
	t.Parallel()
	declining := &workerfake.Server{WorkerName: "worker-1", SupportedFeatures: []string{draftport.Feature}, DraftFunc: workerfake.DeclineDraft("cases disagree")}
	reply, err := remote.NewDrafter(workerfake.ConnectClient(t, declining), "worker-1", "epoch").Draft(t.Context(), draftRequest())
	if err != nil || reply.Proposal != nil || reply.NoProposal != "cases disagree" {
		t.Fatalf("declined draft = %+v, %v", reply, err)
	}
	unable := &workerfake.Server{WorkerName: "worker-1"}
	if _, err := remote.NewDrafter(workerfake.ConnectClient(t, unable), "worker-1", "epoch").Draft(t.Context(), draftRequest()); !errors.Is(err, draftport.ErrUnsupported) {
		t.Fatalf("a worker without the feature = %v, want unsupported", err)
	}
}
