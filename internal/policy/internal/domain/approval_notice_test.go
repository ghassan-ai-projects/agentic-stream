package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
)

func TestApprovalNoticeRetainsEvidenceAndFallbacks(t *testing.T) {
	t.Parallel()
	row := IntentRecord{TenantID: "tenant", IntentID: "intent", DecisionID: "decision", SituationID: "situation", SituationVersion: 1, RiskClass: "R2", IntentSHA: make([]byte, 32)}
	intent := map[string]any{"evidence_ids": []any{"one", "", 1, "two"}, "parameters": nil}
	context := ApprovalContext{Snapshot: make([]byte, 32), Delta: map[string]any{"change": 1}, Decision: DecisionDocument{Summary: "  ", Hypothesis: "motor wear"}, Source: "source"}
	data := BuildApprovalNotification(ApprovalNotice{Row: row, ID: "approval", ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), Intent: ProjectIntent(intent), Context: context})
	if data.Summary != "Decision decision requires approval" || data.Hypothesis != "motor wear" || data.SourceAuthority != "source" {
		t.Fatal(data)
	}
	evidence := data.Evidence
	if len(evidence) != 2 || evidence[0] != "one" || evidence[1] != "two" {
		t.Fatal(evidence)
	}
	if _, ok := intent["parameters"].(map[string]any); !ok {
		t.Fatal("parameters not filled")
	}
	raw, err := ApprovalRequestJSON(row, ProjectIntent(intent), ApprovalRequest{ID: "approval", Nonce: "nonce", Data: data})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if document["nonce"] != "nonce" || document["notification"].(map[string]any)["hypothesis"] != "motor wear" {
		t.Fatal(document)
	}
}

func TestApprovalNotificationKeepsItsSealedJSONShape(t *testing.T) {
	t.Parallel()
	row := IntentRecord{TenantID: "tenant", IntentID: "intent", DecisionID: "decision", SituationID: "situation", SituationVersion: 2, RiskClass: "R2", IntentSHA: make([]byte, 32)}
	notice := ApprovalNotice{Row: row, ID: "approval", ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), Intent: ProjectIntent(map[string]any{}), Context: ApprovalContext{Snapshot: make([]byte, 32), Decision: DecisionDocument{Summary: "s", Hypothesis: "h"}, Source: "source"}}
	raw, err := canonicaljson.Marshal(BuildApprovalNotification(notice))
	if err != nil {
		t.Fatal(err)
	}
	zero := "sha256:" + strings.Repeat("0", 64)
	want := `{"action":{},"approval_id":"approval","audience":"stream-approval-relay","decision_id":"decision","decline_consequence":"The intent will not be dispatched.","delta":{},"evidence":[],"expires_at":"2099-01-01T00:00:00.000000000Z","hypothesis":"h","intent_digest":"` + zero + `","intent_id":"intent","risk_class":"R2","situation_id":"situation","situation_version":2,"snapshot_digest":"` + zero + `","source_authority":"source","summary":"s","tenant_id":"tenant"}`
	if string(raw) != want {
		t.Fatalf("sealed notification changed:\n got %s\nwant %s", raw, want)
	}
}

func TestAnApprovalNotificationMustBeBoundToItsTenantAndSource(t *testing.T) {
	t.Parallel()
	notification := ApprovalNotification{TenantID: "tenant", SourceAuthority: "source"}
	tests := []struct {
		name           string
		tenant, source string
		wantRefused    bool
	}{
		{"bound", "tenant", "source", false},
		{"another tenant", "other", "source", true},
		{"another source authority", "tenant", "elsewhere", true},
	}
	for _, test := range tests {
		err := notification.CheckBinding(test.tenant, test.source)
		if (err != nil) != test.wantRefused {
			t.Errorf("%s: CheckBinding = %v, want refused=%t", test.name, err, test.wantRefused)
		}
	}
}
