package domain

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1/contractstest"
	"github.com/ghassan-ai-projects/agentic-stream/internal/episodes"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

func TestEpisodeKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    runtimev1.EpisodeKind
		wantErr bool
	}{
		{name: "standard", value: "standard", want: runtimev1.EpisodeKind_EPISODE_KIND_DIAGNOSE},
		{name: "diagnose", value: "diagnose", want: runtimev1.EpisodeKind_EPISODE_KIND_DIAGNOSE},
		{name: "reconsider", value: "reconsider", want: runtimev1.EpisodeKind_EPISODE_KIND_RECONSIDER},
		{name: "invalid", value: "unsupported", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := episodeKind(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatal("episodeKind() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("episodeKind() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("episodeKind() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEpisodeRequestMapsReconsiderationPayload(t *testing.T) {
	t.Parallel()

	req := validWorkerRequest()
	var payload map[string]any
	if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
		t.Fatalf("decode request fixture: %v", err)
	}
	payload["kind"] = "reconsider"
	payload["reconsideration"] = map[string]any{
		"prior_decision": map[string]any{"decision_id": "dec-prior"},
		"commands":       []map[string]any{{"command_id": "cmd-prior", "intent_type": "maintenance.ticket", "status": "succeeded"}},
		"outcomes":       []map[string]any{{"outcome_id": "out-prior", "command_id": "cmd-prior", "status": "succeeded"}},
		"correction":     map[string]any{"invalidates": []string{"cmd-prior"}, "superseded_version": 1},
	}
	encoded, err := canonicaljson.Marshal(payload)
	if err != nil {
		t.Fatalf("encode request fixture: %v", err)
	}
	req.RequestJSON = encoded

	wire, err := WireRequest(req)
	if err != nil {
		t.Fatalf("build worker request: %v", err)
	}
	if wire.GetKind() != runtimev1.EpisodeKind_EPISODE_KIND_RECONSIDER {
		t.Fatalf("wire kind = %v, want reconsider", wire.GetKind())
	}
	if wire.GetReconsideration() == nil {
		t.Fatal("expected wire reconsideration payload")
	}
	if got := string(wire.GetReconsideration().GetPriorDecisionJson()); got != `{"decision_id":"dec-prior"}` {
		t.Fatalf("prior decision json = %s", got)
	}
	if len(wire.GetReconsideration().GetExecutedCommandJson()) != 1 ||
		string(wire.GetReconsideration().GetExecutedCommandJson()[0]) != `{"command_id":"cmd-prior","intent_type":"maintenance.ticket","status":"succeeded"}` {
		t.Fatalf("executed command json = %s", wire.GetReconsideration().GetExecutedCommandJson())
	}
}

func TestEpisodeRequestMapsWatchConfidenceFloor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   *float64
		wantSet bool
		want    float64
	}{
		{name: "omitted", wantSet: false},
		{name: "custom", value: func() *float64 { v := 0.7; return &v }(), wantSet: true, want: 0.7},
		{name: "opt out", value: func() *float64 { v := 0.0; return &v }(), wantSet: true, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := validWorkerRequest()
			var payload map[string]any
			if err := json.Unmarshal(req.RequestJSON, &payload); err != nil {
				t.Fatalf("decode request fixture: %v", err)
			}
			if tt.value == nil {
				delete(payload, "watch_confidence_floor")
			} else {
				payload["watch_confidence_floor"] = *tt.value
			}
			encoded, err := canonicaljson.Marshal(payload)
			if err != nil {
				t.Fatalf("encode request fixture: %v", err)
			}
			req.RequestJSON = encoded

			wire, err := WireRequest(req)
			if err != nil {
				t.Fatalf("build worker request: %v", err)
			}
			if (wire.WatchConfidenceFloor != nil) != tt.wantSet {
				t.Fatalf("watch confidence floor presence = %v, want %v", wire.WatchConfidenceFloor != nil, tt.wantSet)
			}
			if tt.wantSet && wire.GetWatchConfidenceFloor() != tt.want {
				t.Fatalf("watch confidence floor = %v, want %v", wire.GetWatchConfidenceFloor(), tt.want)
			}
		})
	}
}

func validWorkerRequest() *episodes.Request {
	promptDigest, _ := canonicaljson.Digest(canonicaljson.DomainPrompt, map[string]any{"version": "prompt-v1"})
	objectiveDigest, _ := canonicaljson.Digest(canonicaljson.DomainObjective, map[string]any{"text": "diagnose"})
	intentCatalog, intentDigest, err := episodes.CompileIntentCatalog([]spec.Intent{
		{Type: "create_ticket", Risk: "R1", ParameterSchema: ticketSchema()},
	})
	if err != nil {
		panic(err)
	}
	intentCatalogJSON, _ := json.Marshal(intentCatalog)
	return &episodes.Request{
		EpisodeID: "episode-1", TenantID: "tenant-1", SituationID: "situation-1", SituationVersion: 1, EntityID: "motor-1",
		ExecutorName: "worker", ExecutorVersion: "sha256:" + "00" + "00000000000000000000000000000000000000000000000000000000000000",
		PromptVersion: "prompt-v1", SnapshotSHA256: "sha256:" + "00" + "00000000000000000000000000000000000000000000000000000000000000",
		AttemptID: "attempt-1", Fence: 7, Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		PromptSHA256: promptDigest, ObjectiveSHA256: objectiveDigest,
		RequestJSON: []byte(fmt.Sprintf(`{"kind":"diagnose","snapshot":{"situation_id":"situation-1"},"tools":[],"risk_ceiling":"R1","trigger":{"trigger_id":"trigger-1","lane":"fast"},"executor":{"objective":"diagnose","prompt_sha256":%q,"objective_sha256":%q,"decision_schema":{"type":"object"},"intent_catalog":%s,"intent_catalog_sha256":%q},"budget":{"wall_time":"1m"}}`, promptDigest, objectiveDigest, string(intentCatalogJSON), intentDigest)),
	}
}

func ticketSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"entity_id": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}}
}

func TestRiskCeilingMapsEveryClassAndNothingElse(t *testing.T) {
	t.Parallel()
	want := []runtimev1.RiskClass{
		runtimev1.RiskClass_RISK_CLASS_R0, runtimev1.RiskClass_RISK_CLASS_R1, runtimev1.RiskClass_RISK_CLASS_R2,
		runtimev1.RiskClass_RISK_CLASS_R3, runtimev1.RiskClass_RISK_CLASS_R4,
	}
	for index, class := range contractstest.RiskClasses() {
		got, err := riskClass(string(class))
		if err != nil || got != want[index] {
			t.Fatalf("%s: got %v, %v", class, got, err)
		}
	}
	for _, value := range []string{"", "r1", " R1 ", "R5"} {
		if _, err := riskClass(value); err == nil {
			t.Fatalf("%q accepted", value)
		}
	}
}
