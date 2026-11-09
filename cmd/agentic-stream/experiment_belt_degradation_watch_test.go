package main

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/campaign"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/workerfake"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

//nolint:paralleltest // serve reads the approval token and relay from the process environment (t.Setenv)
func TestAReasonerThatOnlyWatchesWhileTheCauseIsUnknownTakesTheStepWhenTheWatchFires(t *testing.T) {
	t.Setenv("AGENTIC_STREAM_APPROVAL_TOKEN", conveyorApprovalToken)
	t.Setenv("AGENTIC_STREAM_APPROVAL_RELAY", conveyorRelay)
	specPath := compressedSpec(t, nil)
	run := startDegradationServe(t, specPath, "a", watchWhileUnsure(compressedCoverage, causeWatch(t, specPath)))
	desk := approvalDesk(run.address, conveyorApprovalToken, conveyorApprover, campaign.ApproveAll{})
	line := connectBeltLine(t, run, bearingDegradation)
	settled := run.approvingUntilSettled(t, desk, len(isolationRun.order)+1, "isolated")
	if !line.runUntil(t, 2*degradationTicks, settled) || !slices.Equal(line.obeyed, isolationRun.order) {
		t.Fatalf("after %d minutes the host commanded %v, want %v", line.minutes(), line.obeyed, isolationRun.order)
	}
	assertPhases(t, run.db, []string{"normal", "watch", "degrading", "shedding", "cause_bearing", "isolated"})
	if fires := countRows(t, run.db, "watch_fires"); fires != 1 {
		t.Fatalf("%d watch fires, want 1: the watch fires once, when the bearing rule starts to hold", fires)
	}
	query := `SELECT COUNT(*) FROM situation_versions WHERE phase = 'degrading'`
	if versions := columnValues(t, run.db, query); len(versions) != 1 || versions[0] != "2" {
		t.Fatalf("degrading versions = %v, want 2: the phase change and the version the watch woke", versions)
	}
}

func causeWatch(t *testing.T, specPath string) proposal {
	t.Helper()
	for _, intent := range compileConveyorSpec(t, specPath).Actions.Intents {
		if intent.Type != "install_watch_condition" {
			continue
		}
		preset := intent.Presets["default"]
		return proposal{Type: intent.Type, Risk: "R0", Parameters: map[string]any{"expression": preset["expression"], "max_fires": preset["max_fires"],
			"metric": preset["metric"], "threshold": preset["threshold"], "target": "conveyor-1"}}
	}
	t.Fatalf("%s has no install_watch_condition intent", specPath)
	return proposal{}
}

//nolint:paralleltest // serve reads the approval token and relay from the process environment (t.Setenv)
func TestAnEpisodeWhoseDecisionIsRejectedIsRetriedAsANewEpisodeAfterTheEventTimeBackoff(t *testing.T) {
	t.Setenv("AGENTIC_STREAM_APPROVAL_TOKEN", conveyorApprovalToken)
	t.Setenv("AGENTIC_STREAM_APPROVAL_RELAY", conveyorRelay)
	run := startDegradationServe(t, compressedSpec(t, nil), "a", rejectTheFirstEpisode(reasonAboutTheLine(compressedCoverage)))
	desk := approvalDesk(run.address, conveyorApprovalToken, conveyorApprover, campaign.ApproveAll{})
	line := connectBeltLine(t, run, bearingDegradation)
	if !line.runUntil(t, 2*degradationTicks, run.approvingUntilSettled(t, desk, len(isolationRun.order), "isolated")) || !slices.Equal(line.obeyed, isolationRun.order) {
		t.Fatalf("after %d minutes the host commanded %v, want %v", line.minutes(), line.obeyed, isolationRun.order)
	}
	episodes := columnValues(t, run.db, `SELECT v.phase || ' ' || e.lifecycle_status || ' ' || COALESCE((SELECT GROUP_CONCAT(d.validation_status) FROM decisions d WHERE d.episode_id = e.episode_id), '')
		FROM episodes e JOIN situation_versions v ON v.situation_id = e.situation_id AND v.version = e.situation_version ORDER BY e.accepted_at, e.episode_id`)
	if len(episodes) < 2 || episodes[0] != "degrading concluded rejected" || !strings.HasPrefix(episodes[1], "degrading concluded accepted") {
		t.Fatalf("episodes = %v, want the rejected degrading episode followed by a new, accepted one", episodes)
	}
}

func rejectTheFirstEpisode(reasoner workerfake.ExecuteFunc) workerfake.ExecuteFunc {
	var mu sync.Mutex
	first := ""
	return func(ctx context.Context, req *runtimev1.EpisodeRequest, emit func(*runtimev1.EpisodeEvent) error) error {
		mu.Lock()
		first = cmp.Or(first, req.GetEpisodeId())
		rejected := first == req.GetEpisodeId()
		mu.Unlock()
		if rejected {
			return workerfake.Propose(req, emit, workerfake.Answer{Hypothesis: unknownCause, DiagnosisCode: unknownCause, Summary: "no level",
				Intents: []proposal{{Type: "reduce_feed", Risk: "R1", Parameters: map[string]any{}}}})
		}
		return reasoner(ctx, req, emit)
	}
}
