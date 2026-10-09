package domain

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/executorconformance"
	"github.com/ghassan-ai-projects/agentic-stream/internal/worker"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func executorSection(payload map[string]any) map[string]any {
	section, _ := payload["executor"].(map[string]any)
	return section
}

func reconsiderationDocument() map[string]any {
	return map[string]any{
		"prior_decision": map[string]any{"decision_id": "dec-prior"},
		"commands":       []any{map[string]any{"command_id": "cmd-prior", "intent_type": "maintenance.ticket", "status": "succeeded"}},
		"outcomes":       []any{map[string]any{"outcome_id": "out-prior", "command_id": "cmd-prior", "status": "succeeded"}},
		"correction":     map[string]any{"invalidates": []any{"cmd-prior"}, "superseded_version": 1},
	}
}

func reconsider(edit func(document map[string]any)) func(map[string]any) {
	return func(payload map[string]any) {
		document := reconsiderationDocument()
		if edit != nil {
			edit(document)
		}
		payload["kind"] = "reconsider"
		payload["reconsideration"] = document
	}
}

func TestWireRequestBindsTheDurableEpisodeToTheWorkerRequest(t *testing.T) {
	t.Parallel()
	req := newRequest(func(payload map[string]any) {
		payload["budget"] = map[string]any{
			"wall_time": "5s", "model_calls": 4, "input_tokens": 100, "output_tokens": 50, "tool_calls": 3,
			"tool_result_bytes": 1024, "total_tool_result_bytes": 4096, "provider_retries": 2, "cost_microunits": 900,
		}
		payload["cancellation_key"], payload["supersession_key"] = "cancel-1", "supersede-1"
	})
	req.Traceparent, req.Tracestate, req.DispatchPolicy = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "vendor=1", "active"

	wire, err := WireRequest(req)
	if err != nil {
		t.Fatalf("WireRequest() = %v", err)
	}

	snapshotDigest, _ := canonicaljson.DecodeDigest(req.SnapshotSHA256)
	identity := []struct{ field, got, want any }{
		{"protocol version", wire.GetProtocolVersion(), worker.ProtocolVersion},
		{"episode", wire.GetEpisodeId(), req.EpisodeID},
		{"attempt", wire.GetAttemptId(), req.AttemptID},
		{"fence", wire.GetFence(), uint64(1)},
		{"tenant", wire.GetTenantId(), req.TenantID},
		{"situation", wire.GetSituationId(), req.SituationID},
		{"situation version", wire.GetSituationVersion(), uint64(1)},
		{"trigger", wire.GetTriggerId(), "trg-1"},
		{"kind", wire.GetKind(), runtimev1.EpisodeKind_EPISODE_KIND_DIAGNOSE},
		{"lane", wire.GetLane(), runtimev1.EpisodeLane_EPISODE_LANE_DEEP},
		{"risk ceiling", wire.GetRiskCeiling(), runtimev1.RiskClass_RISK_CLASS_R1},
		{"dispatch policy", wire.GetDispatchPolicy(), runtimev1.DispatchPolicy_DISPATCH_POLICY_ACTIVE},
		{"traceparent", wire.GetTraceparent(), req.Traceparent},
		{"tracestate", wire.GetTracestate(), req.Tracestate},
		{"cancellation key", wire.GetCancellationKey(), "cancel-1"},
		{"supersession key", wire.GetSupersessionKey(), "supersede-1"},
		{"objective", wire.GetObjective(), "diagnose"},
		{"executor name", wire.GetExecutorName(), req.ExecutorName},
		{"skill refs", string(wire.GetSkillRefsJson()), "[]"},
	}
	for _, c := range identity {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.field, c.got, c.want)
		}
	}
	if !bytes.Equal(wire.GetSnapshotSha256(), snapshotDigest) || len(wire.GetSpecSha256()) != 32 || len(wire.GetPromptSha256()) != 32 || len(wire.GetObjectiveSha256()) != 32 {
		t.Errorf("provenance digests = snapshot %x spec %d prompt %d objective %d", wire.GetSnapshotSha256(), len(wire.GetSpecSha256()), len(wire.GetPromptSha256()), len(wire.GetObjectiveSha256()))
	}
	if len(wire.GetAllowedIntentTypes()) != 1 || wire.GetAllowedIntentTypes()[0] != "create_maintenance_ticket" || !strings.Contains(string(wire.GetIntentCatalogJson()), "create_maintenance_ticket") {
		t.Errorf("intent surface = %v / %s", wire.GetAllowedIntentTypes(), wire.GetIntentCatalogJson())
	}
	budget := wire.GetBudget()
	if budget.GetWallTime().AsDuration() != 5*time.Second || budget.GetMaxModelCalls() != 4 || budget.GetMaxInputTokens() != 100 || budget.GetMaxOutputTokens() != 50 ||
		budget.GetMaxToolCalls() != 3 || budget.GetMaxToolResultBytes() != 1024 || budget.GetMaxTotalToolResultBytes() != 4096 || budget.GetMaxProviderRetries() != 2 || budget.GetMaxCostMicrounits() != 900 {
		t.Errorf("budget = %v", budget)
	}
}

func TestWireRequestRejectsWhatCannotBeBoundToTheEpisode(t *testing.T) {
	t.Parallel()
	payloadFault := func(edit func(payload map[string]any)) func(*episodes.Request) {
		return func(req *episodes.Request) { executorconformance.EditPayload(req, edit) }
	}
	tests := []struct {
		name  string
		fault func(*episodes.Request)
		want  string
	}{
		{"situation version", func(r *episodes.Request) { r.SituationVersion = 0 }, "situation version must be positive"},
		{"fence", func(r *episodes.Request) { r.Fence = 0 }, "fence must be positive"},
		{"request json", func(r *episodes.Request) { r.RequestJSON = []byte("{") }, "decode request json"},
		{"snapshot", payloadFault(func(p map[string]any) { delete(p, "snapshot") }), "snapshot is required"},
		{"tool catalog", payloadFault(func(p map[string]any) { p["tools"] = nil }), "tools is required"},
		{"decision schema", payloadFault(func(p map[string]any) { delete(executorSection(p), "decision_schema") }), "decision_schema is required"},
		{"snapshot digest", func(r *episodes.Request) { r.SnapshotSHA256 = "broken" }, "snapshot digest"},
		{"spec digest", func(r *episodes.Request) { r.ExecutorVersion = "broken" }, "spec digest"},
		{"missing prompt digest", payloadFault(func(p map[string]any) { delete(executorSection(p), "prompt_sha256") }), "provenance digests are required"},
		{"missing durable objective digest", func(r *episodes.Request) { r.ObjectiveSHA256 = "" }, "provenance digests are required"},
		{"malformed prompt digest", func(r *episodes.Request) {
			r.PromptSHA256 = "broken"
			executorconformance.EditPayload(r, func(p map[string]any) { executorSection(p)["prompt_sha256"] = "broken" })
		}, "prompt digest"},
		{"malformed objective digest", func(r *episodes.Request) {
			r.ObjectiveSHA256 = "broken"
			executorconformance.EditPayload(r, func(p map[string]any) { executorSection(p)["objective_sha256"] = "broken" })
		}, "objective digest"},
		{"malformed diagnosis catalog digest", payloadFault(func(p map[string]any) { executorSection(p)["diagnosis_catalog_sha256"] = "broken" }), "diagnosis catalog digest"},
		{"prompt provenance differs from the durable episode", payloadFault(func(p map[string]any) { executorSection(p)["prompt_sha256"] = "sha256:" + strings.Repeat("0", 64) }), "does not match durable episode provenance"},
		{"episode kind", payloadFault(func(p map[string]any) { p["kind"] = "unsupported" }), `unsupported episode kind "unsupported"`},
		{"episode lane", payloadFault(func(p map[string]any) { p["trigger"].(map[string]any)["lane"] = "batch" }), `unsupported episode lane "batch"`},
		{"risk ceiling", payloadFault(func(p map[string]any) { p["risk_ceiling"] = "R9" }), `unsupported risk ceiling "R9"`},
		{"wall time", payloadFault(func(p map[string]any) { p["budget"] = map[string]any{"wall_time": "broken"} }), "validate episode budget"},
		{"empty intent catalog", payloadFault(func(p map[string]any) { executorSection(p)["intent_catalog"] = []any{} }), "intent catalog is empty"},
		{"reconsideration payload", payloadFault(func(p map[string]any) { p["kind"] = "reconsider" }), "reconsideration payload: payload is required"},
		{"reconsideration prior decision", payloadFault(reconsider(func(d map[string]any) { delete(d, "prior_decision") })), "prior_decision is required"},
		{"reconsideration correction", payloadFault(reconsider(func(d map[string]any) { d["correction"] = nil })), "correction is required"},
		{"reconsideration command", payloadFault(reconsider(func(d map[string]any) { d["commands"] = []any{nil} })), "commands[0] is required"},
		{"reconsideration outcome", payloadFault(reconsider(func(d map[string]any) { d["outcomes"] = []any{map[string]any{}, nil} })), "outcomes[1] is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := newRequest()
			tt.fault(req)
			wire, err := WireRequest(req)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("WireRequest() = %v, %v; want error containing %q", wire, err, tt.want)
			}
		})
	}
}

func TestWireRequestReportsTheEarliestFaultOfSeveral(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		fault func(*episodes.Request)
		want  string
	}{
		{"documents before provenance, shape and budget", func(r *episodes.Request) {
			executorconformance.EditPayload(r, func(p map[string]any) {
				delete(p, "snapshot")
				p["kind"] = "unsupported"
				p["budget"] = map[string]any{"wall_time": "broken"}
			})
			r.SnapshotSHA256 = "broken"
		}, "snapshot is required"},
		{"provenance before shape and budget", func(r *episodes.Request) {
			executorconformance.EditPayload(r, func(p map[string]any) {
				p["kind"] = "unsupported"
				p["budget"] = map[string]any{"wall_time": "broken"}
			})
			r.SnapshotSHA256 = "broken"
		}, "snapshot digest"},
		{"shape before budget", func(r *episodes.Request) {
			executorconformance.EditPayload(r, func(p map[string]any) {
				p["kind"] = "unsupported"
				p["budget"] = map[string]any{"wall_time": "broken"}
			})
		}, "unsupported episode kind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := newRequest()
			tt.fault(req)
			if _, err := WireRequest(req); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("WireRequest() = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestWireRequestMapsReconsiderationEvidence(t *testing.T) {
	t.Parallel()
	req := newRequest(reconsider(nil))
	wire, err := WireRequest(req)
	if err != nil {
		t.Fatalf("WireRequest() = %v", err)
	}
	if wire.GetKind() != runtimev1.EpisodeKind_EPISODE_KIND_RECONSIDER {
		t.Fatalf("kind = %v, want reconsider", wire.GetKind())
	}
	evidence := wire.GetReconsideration()
	if got := string(evidence.GetPriorDecisionJson()); got != `{"decision_id":"dec-prior"}` {
		t.Errorf("prior decision = %s", got)
	}
	if len(evidence.GetExecutedCommandJson()) != 1 || string(evidence.GetExecutedCommandJson()[0]) != `{"command_id":"cmd-prior","intent_type":"maintenance.ticket","status":"succeeded"}` {
		t.Errorf("executed commands = %s", evidence.GetExecutedCommandJson())
	}
	if len(evidence.GetObservedOutcomeJson()) != 1 || !strings.Contains(string(evidence.GetObservedOutcomeJson()[0]), "out-prior") {
		t.Errorf("observed outcomes = %s", evidence.GetObservedOutcomeJson())
	}
	if !strings.Contains(string(evidence.GetCorrectionJson()), "superseded_version") {
		t.Errorf("correction = %s", evidence.GetCorrectionJson())
	}
}

func TestWireRequestMapsTheWatchConfidenceFloor(t *testing.T) {
	t.Parallel()
	custom, optOut := 0.7, 0.0
	tests := []struct {
		name    string
		value   *float64
		wantSet bool
		want    float64
	}{
		{name: "omitted"},
		{name: "custom", value: &custom, wantSet: true, want: 0.7},
		{name: "opt out", value: &optOut, wantSet: true, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := newRequest(func(payload map[string]any) {
				if tt.value != nil {
					payload["watch_confidence_floor"] = *tt.value
				}
			})
			wire, err := WireRequest(req)
			if err != nil {
				t.Fatalf("WireRequest() = %v", err)
			}
			if (wire.WatchConfidenceFloor != nil) != tt.wantSet || wire.GetWatchConfidenceFloor() != tt.want {
				t.Fatalf("watch confidence floor = %v, want set=%v value=%v", wire.WatchConfidenceFloor, tt.wantSet, tt.want)
			}
		})
	}
}
