package eventlog_test

import (
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
)

func TestQuarantinedEnvelopeIsListedReleasedAndRedrivenOnceThroughTheFacade(t *testing.T) {
	t.Parallel()
	log, _ := newLog(t)
	event := envelope("evt-1", "default")
	if err := log.QuarantineEnvelope(t.Context(), "default", event, "operator_hold", noon); err != nil {
		t.Fatal(err)
	}
	if err := log.QuarantineRaw(t.Context(), "default", "line-1", []byte("not-json"), "malformed_json", noon); err != nil {
		t.Fatal(err)
	}
	records, err := log.Quarantined(t.Context(), "default")
	if err != nil || len(records) != 2 {
		t.Fatalf("records = %+v err=%v, want the envelope and the raw line", records, err)
	}
	if err := log.ReleaseQuarantine(t.Context(), "default", "evt-1", noon.Add(1)); err != nil {
		t.Fatal(err)
	}
	position, err := log.RedriveQuarantine(t.Context(), "default", "evt-1", noon.Add(2))
	if err != nil || position <= 0 {
		t.Fatalf("redrive position = %d err=%v, want a log position", position, err)
	}
	if ids := readIDs(t, log, eventlog.ReadRequest{TenantID: "default", PartitionID: -1}); len(ids) != 1 || ids[0] != "evt-1" {
		t.Fatalf("log = %v, want the redriven evt-1", ids)
	}
	if _, err := log.RedriveQuarantine(t.Context(), "default", "evt-1", noon.Add(3)); err == nil {
		t.Fatal("a second redrive was accepted")
	}
	statuses := map[string]string{}
	records, err = log.Quarantined(t.Context(), "default")
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		statuses[record.EventID] = record.Status
	}
	if statuses["evt-1"] != "redriven" || statuses["line-1"] != "quarantined" {
		t.Fatalf("statuses = %v, want evt-1 redriven and line-1 quarantined", statuses)
	}
}
