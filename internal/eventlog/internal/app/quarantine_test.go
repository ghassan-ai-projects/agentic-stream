package app

import (
	"strings"
	"testing"
	"time"
)

func (h harness) quarantine(t *testing.T, eventID string, version int) error {
	t.Helper()
	return h.service.Quarantine(t.Context(), "tenant", map[string]any{"id": eventID, "v": version}, "malformed_json", noon)
}

func (h harness) gaps(t *testing.T) int {
	t.Helper()
	var gaps int
	if err := h.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM event_gaps WHERE reason_code = 'quarantine_retry_exhausted'").Scan(&gaps); err != nil {
		t.Fatal(err)
	}
	return gaps
}

func TestQuarantineCountsRetriesAndRejectsOnceTheBoundIsSpent(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for delivery := 1; delivery <= 10; delivery++ {
		if err := h.quarantine(t, "poison", 1); err != nil {
			t.Fatalf("delivery %d: %v", delivery, err)
		}
	}
	if record := h.quarantined(t)[0]; record.Status != "quarantined" || record.AttemptCount != 10 || h.gaps(t) != 0 {
		t.Fatalf("after 10 deliveries: %+v gaps=%d, want quarantined with no gap yet", record, h.gaps(t))
	}
	for range 3 {
		if err := h.quarantine(t, "poison", 1); err != nil {
			t.Fatal(err)
		}
	}
	if record := h.quarantined(t)[0]; record.Status != "rejected" || record.AttemptCount != 10 {
		t.Fatalf("after the bound: %+v, want rejected at 10 attempts", record)
	}
	if gaps := h.gaps(t); gaps != 1 {
		t.Fatalf("overflow gaps = %d, want exactly one however many deliveries follow", gaps)
	}
}

func TestQuarantineReportsADifferentPayloadUnderTheSameEventIDAfterRejectingTheRecord(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := h.quarantine(t, "bad-1", 1); err != nil {
		t.Fatal(err)
	}
	err := h.quarantine(t, "bad-1", 2)
	if err == nil || !strings.Contains(err.Error(), "event id bad-1 has conflicting quarantined payload") {
		t.Fatalf("err = %v, want the conflicting payload error", err)
	}
	record := h.quarantined(t)[0]
	if record.Status != "rejected" || record.ReasonCode != "event_id_hash_conflict" {
		t.Fatalf("record = %+v, want the rejection committed before the error was reported", record)
	}
}

func TestQuarantineRequiresTenantReasonAndTime(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	payload := map[string]any{"id": "bad"}
	cases := []struct {
		name   string
		tenant string
		reason string
		at     time.Time
	}{
		{"tenant", "", "malformed_json", noon},
		{"reason", "tenant", "", noon},
		{"time", "tenant", "malformed_json", time.Time{}},
	}
	for _, tc := range cases {
		if err := h.service.Quarantine(t.Context(), tc.tenant, payload, tc.reason, tc.at); err == nil || !strings.Contains(err.Error(), "are required") {
			t.Errorf("quarantine without %s: err = %v, want the required-fields error", tc.name, err)
		}
	}
	if records := h.quarantined(t); len(records) != 0 {
		t.Fatalf("a refused quarantine stored %+v", records)
	}
}

func TestQuarantineRefusesAPayloadItCannotEncode(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	err := h.service.Quarantine(t.Context(), "tenant", map[string]any{"bad": make(chan int)}, "malformed_json", noon)
	if err == nil || !strings.Contains(err.Error(), "marshal quarantined payload") {
		t.Fatalf("err = %v, want marshal quarantined payload", err)
	}
	bad := envelope("evt-bad")
	bad.Data = map[string]any{"bad": make(chan int)}
	if err := h.service.QuarantineEnvelope(t.Context(), "tenant", bad, "schema_mismatch", noon); err == nil || !strings.Contains(err.Error(), "marshal quarantined envelope") {
		t.Fatalf("envelope err = %v, want marshal quarantined envelope", err)
	}
}

func TestQuarantineRawKeepsTheBytesAsDataUnderTheGivenEventID(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := h.service.QuarantineRaw(t.Context(), "tenant", "line-1", []byte("not-json"), "malformed_json", noon); err != nil {
		t.Fatal(err)
	}
	var payload string
	if err := h.db.QueryRowContext(t.Context(), "SELECT payload_json FROM event_quarantine WHERE event_id = 'line-1'").Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"raw":"not-json"`) {
		t.Fatalf("payload = %s, want the raw bytes kept as the data.raw string", payload)
	}
	if records := h.quarantined(t); len(records) != 1 || records[0].EventID != "line-1" {
		t.Fatalf("records = %+v, want one record for line-1", records)
	}
}

func TestReleasedEnvelopeIsRedrivenIntoTheLogExactlyOnce(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.service.RequireSchemaValidation()
	h.registerTemperatureSchema(t, temperatureSchema)
	if err := h.service.QuarantineEnvelope(t.Context(), "tenant", envelope("evt-1"), "operator_hold", noon); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.RedriveQuarantine(t.Context(), "tenant", "evt-1", noon); err == nil || !strings.Contains(err.Error(), "is not released") {
		t.Fatalf("redrive before release: err = %v, want is not released", err)
	}
	if err := h.service.ReleaseQuarantine(t.Context(), "tenant", "evt-1", noon.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	position, err := h.service.RedriveQuarantine(t.Context(), "tenant", "evt-1", noon.Add(2*time.Minute))
	if err != nil || position <= 0 {
		t.Fatalf("redrive position = %d err=%v, want a log position", position, err)
	}
	if records := h.readAll(t); len(records) != 1 || records[0].EventID != "evt-1" {
		t.Fatalf("log = %+v, want the redriven event", records)
	}
	if record := h.quarantined(t)[0]; record.Status != "redriven" {
		t.Fatalf("status = %q, want redriven", record.Status)
	}
	if _, err := h.service.RedriveQuarantine(t.Context(), "tenant", "evt-1", noon.Add(3*time.Minute)); err == nil || !strings.Contains(err.Error(), "is not released") {
		t.Fatalf("second redrive: err = %v, want is not released", err)
	}
	if got := len(h.readAll(t)); got != 1 {
		t.Fatalf("log holds %d records after the second redrive, want 1", got)
	}
}

func TestRedriveOfAnEnvelopeThatStillFailsAdmissionLeavesTheLogAndTheReleaseUntouched(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.service.RequireSchemaValidation()
	h.registerTemperatureSchema(t, temperatureSchema)
	bad := envelope("evt-bad")
	bad.Data = map[string]any{"celsius": "hot"}
	if err := h.service.QuarantineEnvelope(t.Context(), "tenant", bad, "schema_mismatch", noon); err != nil {
		t.Fatal(err)
	}
	if err := h.service.ReleaseQuarantine(t.Context(), "tenant", "evt-bad", noon); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.RedriveQuarantine(t.Context(), "tenant", "evt-bad", noon); err == nil || !strings.Contains(err.Error(), "validate released envelope") {
		t.Fatalf("err = %v, want validate released envelope", err)
	}
	if records := h.readAll(t); len(records) != 0 {
		t.Fatalf("a refused redrive appended %+v", records)
	}
	if record := h.quarantined(t)[0]; record.Status != "released" {
		t.Fatalf("status = %q, want the record to stay released for another attempt", record.Status)
	}
}

func TestRedriveOfARawQuarantineIsRefusedByEnvelopeValidation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := h.service.QuarantineRaw(t.Context(), "tenant", "line-1", []byte("not-json"), "malformed_json", noon); err != nil {
		t.Fatal(err)
	}
	if err := h.service.ReleaseQuarantine(t.Context(), "tenant", "line-1", noon); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.RedriveQuarantine(t.Context(), "tenant", "line-1", noon); err == nil || !strings.Contains(err.Error(), "validate released envelope") {
		t.Fatalf("err = %v, want validate released envelope", err)
	}
}

func TestRedriveOfAnAlreadyLoggedEventAppendsNothingAndStillCompletes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.append(t, envelope("evt-1"))
	if err := h.service.QuarantineEnvelope(t.Context(), "tenant", envelope("evt-1"), "operator_hold", noon); err != nil {
		t.Fatal(err)
	}
	if err := h.service.ReleaseQuarantine(t.Context(), "tenant", "evt-1", noon); err != nil {
		t.Fatal(err)
	}
	position, err := h.service.RedriveQuarantine(t.Context(), "tenant", "evt-1", noon)
	if err != nil || position != -1 {
		t.Fatalf("position = %d err=%v, want the duplicate position -1", position, err)
	}
	if got := len(h.readAll(t)); got != 1 {
		t.Fatalf("log holds %d records, want the original only", got)
	}
}

func TestReleaseAndRedriveRequireTenantEventAndTime(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := h.service.ReleaseQuarantine(t.Context(), "", "", time.Time{}); err == nil || !strings.Contains(err.Error(), "are required") {
		t.Fatalf("release err = %v, want the required-fields error", err)
	}
	if _, err := h.service.RedriveQuarantine(t.Context(), "tenant", "", noon); err == nil || !strings.Contains(err.Error(), "are required") {
		t.Fatalf("redrive err = %v, want the required-fields error", err)
	}
}

func TestReleaseOfAnUnknownRecordIsRefused(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := h.service.ReleaseQuarantine(t.Context(), "tenant", "unknown", noon); err == nil || !strings.Contains(err.Error(), "not available for release") {
		t.Fatalf("err = %v, want not available for release", err)
	}
	if _, err := h.service.RedriveQuarantine(t.Context(), "tenant", "unknown", noon); err == nil || !strings.Contains(err.Error(), "load released quarantine") {
		t.Fatalf("redrive err = %v, want load released quarantine", err)
	}
}

func TestEvidenceEventsLocateLoggedEventsOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.append(t, envelope("evt-1"), envelope("evt-2"))
	events, err := h.service.EvidenceEvents(t.Context(), "tenant", []string{"evt-2", "never-logged"})
	if err != nil || len(events) != 1 || events[0].EventID != "evt-2" {
		t.Fatalf("events = %+v err=%v, want evt-2 only", events, err)
	}
}
