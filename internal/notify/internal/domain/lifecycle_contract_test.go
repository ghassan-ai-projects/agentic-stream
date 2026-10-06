package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/canonicaljson"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

const goldensFile = "contracts/notification-goldens-v1.json"

// loadGoldens reads the cross-repository golden events shipped with the contract.
func loadGoldens(t *testing.T) []contractsv1.CloudEvent {
	t.Helper()
	data, err := os.ReadFile(goldensFile)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		ContractID string                   `json:"contract_id"`
		Version    int                      `json:"version"`
		Events     []contractsv1.CloudEvent `json:"events"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document.ContractID != "situation-runtime/notification-contract/v1" || document.Version != 1 {
		t.Fatalf("goldens identity = %q v%d", document.ContractID, document.Version)
	}
	return document.Events
}

func TestGoldenEventsConform(t *testing.T) {
	t.Parallel()
	events := loadGoldens(t)
	if len(events) != len(lifecycleTypes) {
		t.Fatalf("golden event count = %d, want %d", len(events), len(lifecycleTypes))
	}
	for _, event := range events {
		t.Run(event.Type, func(t *testing.T) {
			t.Parallel()
			digest, err := event.ComputeEnvelopeDigest()
			if err != nil {
				t.Fatal(err)
			}
			if event.EnvelopeDigest != digest {
				t.Fatalf("envelope digest = %q, want %q", event.EnvelopeDigest, digest)
			}
			if err := ValidateLifecycle(event); err != nil {
				t.Fatalf("golden event does not conform: %v", err)
			}
		})
	}
}

func TestValidateLifecycleRejectsUnknownVersionAndAuthorityMismatch(t *testing.T) {
	t.Parallel()
	events := loadGoldens(t)
	unknown := events[0]
	unknown.Type = strings.TrimSuffix(unknown.Type, ".v1") + ".v2"
	if err := ValidateLifecycle(unknown); !errors.Is(err, ErrUnknownType) {
		t.Fatalf("unknown notification version error = %v", err)
	}
	unauthorized := reseal(t, events[1], func(data map[string]any) { data["source_authority"] = "//agentic-stream/tenant/other" })
	if err := ValidateLifecycle(unauthorized); err == nil {
		t.Fatal("authority mismatch was accepted")
	}
	foreign := reseal(t, events[1], func(data map[string]any) { data["tenant_id"] = "other" })
	if err := ValidateLifecycle(foreign); err == nil {
		t.Fatal("tenant mismatch was accepted")
	}
}

// reseal copies a golden event, edits its data and recomputes the digest.
func reseal(t *testing.T, event contractsv1.CloudEvent, edit func(map[string]any)) contractsv1.CloudEvent {
	t.Helper()
	data := map[string]any{}
	for key, value := range event.Data.(map[string]any) {
		data[key] = value
	}
	edit(data)
	event.Data = data
	digest, err := event.ComputeEnvelopeDigest()
	if err != nil {
		t.Fatal(err)
	}
	event.EnvelopeDigest = digest
	return event
}

func TestValidateLifecycleRejectsWrongSchemaAndOrphanTracestate(t *testing.T) {
	t.Parallel()
	golden := loadGoldens(t)[0]
	wrongSchema := golden
	wrongSchema.DataSchema = "urn:other"
	if err := ValidateLifecycle(wrongSchema); err == nil {
		t.Fatal("foreign dataschema was accepted")
	}
	notObject := golden
	notObject.Data = "text"
	if err := ValidateLifecycle(notObject); err == nil {
		t.Fatal("non-object data was accepted")
	}
}

// payloadFrom decodes a golden event's data into its typed payload, refusing
// any field the payload does not declare. The tenant and source authority are
// stamped by the domain and are dropped first.
func payloadFrom(t *testing.T, event contractsv1.CloudEvent) Payload {
	t.Helper()
	data := map[string]any{}
	for key, value := range event.Data.(map[string]any) {
		if key != "tenant_id" && key != "source_authority" {
			data[key] = value
		}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]Payload{
		TypeOutcomeRecorded: &OutcomeRecorded{}, TypeOutcomeReconciled: &OutcomeReconciled{},
		TypeApprovalRequested: &ApprovalRequested{}, TypeApprovalWithdrawn: &ApprovalWithdrawn{}, TypeApprovalResolved: &ApprovalResolved{},
		TypeCommandDispatched: &CommandDispatched{}, TypeSituationSuperseded: &SituationSuperseded{}, TypeReconsiderationAdmitted: &ReconsiderationAdmitted{},
	}[event.Type]
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(payload); err != nil {
		t.Fatalf("%s: payload does not match its contract fields: %v", event.Type, err)
	}
	return payload
}

func TestTypedPayloadsReproduceEveryGoldenEvent(t *testing.T) {
	t.Parallel()
	goldens := loadGoldens(t)
	if len(goldens) != len(lifecycleTypes) {
		t.Fatalf("goldens = %d, want %d", len(goldens), len(lifecycleTypes))
	}
	for _, golden := range goldens {
		t.Run(golden.Type, func(t *testing.T) {
			t.Parallel()
			event, err := NewLifecycleEvent(LifecycleEvent{
				ID: golden.ID, TenantID: golden.TenantID, Subject: golden.Subject, PartitionKey: golden.PartitionKey,
				Payload: payloadFrom(t, golden), At: golden.Time, Trace: contractsv1.TraceContext{Traceparent: golden.Traceparent, Tracestate: golden.Tracestate},
			})
			if err != nil {
				t.Fatal(err)
			}
			want, _ := canonicaljson.Marshal(golden.Data)
			got, _ := canonicaljson.Marshal(event.Data)
			if event.Type != golden.Type || event.Source != golden.Source || !bytes.Equal(got, want) {
				t.Fatalf("event type=%s source=%s\n data=%s\nwant=%s", event.Type, event.Source, got, want)
			}
		})
	}
}

func TestNewLifecycleEventSealsInUTC(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 8, 12, 12, 0, 0, 0, time.FixedZone("x", 3600))
	event, err := NewLifecycleEvent(LifecycleEvent{ID: "i", TenantID: "acme", Subject: "s", PartitionKey: "p", At: at, Payload: validSuperseded()})
	if err != nil {
		t.Fatal(err)
	}
	if !event.Time.Equal(at) || event.Time.Location() != time.UTC || !event.IngestedTime.Equal(at) || event.Source != SourceForTenant("acme") {
		t.Fatalf("event = %+v", event)
	}
	if digest, _ := event.ComputeEnvelopeDigest(); digest != event.EnvelopeDigest {
		t.Fatalf("event is not sealed: %q != %q", event.EnvelopeDigest, digest)
	}
	data := event.Data.(map[string]any)
	if data["tenant_id"] != "acme" || data["source_authority"] != SourceForTenant("acme") {
		t.Fatalf("data not bound to the envelope: %v", data)
	}
}

func validSuperseded() SituationSuperseded {
	return SituationSuperseded{SituationID: "s1", SupersededVersion: 1, ReplacementVersion: 2, Reason: "newer_situation_version_admitted"}
}

func TestNewLifecycleEventRefusesIncompleteIdentityBadTraceAndBadPayload(t *testing.T) {
	t.Parallel()
	valid := LifecycleEvent{ID: "i", TenantID: "acme", Subject: "s", PartitionKey: "p", Payload: validSuperseded(), At: time.Now()}
	for name, mutate := range map[string]func(*LifecycleEvent){
		"missing id":        func(r *LifecycleEvent) { r.ID = "" },
		"missing tenant":    func(r *LifecycleEvent) { r.TenantID = "" },
		"missing subject":   func(r *LifecycleEvent) { r.Subject = "" },
		"missing partition": func(r *LifecycleEvent) { r.PartitionKey = "" },
		"missing payload":   func(r *LifecycleEvent) { r.Payload = nil },
		"bad traceparent":   func(r *LifecycleEvent) { r.Trace.Traceparent = "nonsense" },
		"orphan tracestate": func(r *LifecycleEvent) { r.Trace.Tracestate = "a=b" },
		"zero version":      func(r *LifecycleEvent) { r.Payload = SituationSuperseded{SituationID: "s1", Reason: "x"} },
		"empty required":    func(r *LifecycleEvent) { r.Payload = SituationSuperseded{SupersededVersion: 1, ReplacementVersion: 2} },
		"nil approval maps": func(r *LifecycleEvent) { r.Payload = ApprovalRequested{ApprovalID: "a"} },
	} {
		request := valid
		mutate(&request)
		if _, err := NewLifecycleEvent(request); err == nil {
			t.Errorf("%s: request was accepted", name)
		}
	}
	if _, err := NewLifecycleEvent(valid); err != nil {
		t.Fatalf("valid request refused: %v", err)
	}
}
