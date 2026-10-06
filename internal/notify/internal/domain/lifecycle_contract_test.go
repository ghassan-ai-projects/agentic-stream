package domain

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

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

func TestNewLifecycleEventBuildsSealedContractEvent(t *testing.T) {
	t.Parallel()
	golden := loadGoldens(t)[0]
	at := time.Date(2026, 8, 12, 12, 0, 0, 0, time.FixedZone("x", 3600))
	request := LifecycleEvent{
		ID: "life-1", TenantID: golden.TenantID, Type: golden.Type, Subject: golden.Subject, PartitionKey: golden.PartitionKey,
		Data: golden.Data.(map[string]any), At: at, Trace: contractsv1.TraceContext{Traceparent: golden.Traceparent, Tracestate: golden.Tracestate},
	}
	data := map[string]any{}
	for key, value := range request.Data {
		data[key] = value
	}
	data["source_authority"] = SourceForTenant(golden.TenantID)
	request.Data = data
	event, err := NewLifecycleEvent(request)
	if err != nil {
		t.Fatal(err)
	}
	if event.Source != SourceForTenant(golden.TenantID) || !event.Time.Equal(at) || event.Time.Location() != time.UTC || event.DataSchema != SchemaID {
		t.Fatalf("event = %+v", event)
	}
	if digest, _ := event.ComputeEnvelopeDigest(); digest != event.EnvelopeDigest {
		t.Fatalf("event is not sealed: %q != %q", event.EnvelopeDigest, digest)
	}
}

func TestNewLifecycleEventRefusesIncompleteIdentityAndBadTrace(t *testing.T) {
	t.Parallel()
	golden := loadGoldens(t)[0]
	valid := LifecycleEvent{ID: "i", TenantID: golden.TenantID, Type: golden.Type, Subject: "s", PartitionKey: "p", Data: map[string]any{}, At: time.Now()}
	for name, mutate := range map[string]func(*LifecycleEvent){
		"missing id":        func(r *LifecycleEvent) { r.ID = "" },
		"missing tenant":    func(r *LifecycleEvent) { r.TenantID = "" },
		"missing type":      func(r *LifecycleEvent) { r.Type = "" },
		"missing subject":   func(r *LifecycleEvent) { r.Subject = "" },
		"missing partition": func(r *LifecycleEvent) { r.PartitionKey = "" },
		"bad traceparent":   func(r *LifecycleEvent) { r.Trace.Traceparent = "nonsense" },
		"orphan tracestate": func(r *LifecycleEvent) { r.Trace.Tracestate = "a=b" },
		"binding mismatch":  func(r *LifecycleEvent) {},
	} {
		request := valid
		mutate(&request)
		if _, err := NewLifecycleEvent(request); err == nil {
			t.Errorf("%s: request was accepted", name)
		}
	}
}
