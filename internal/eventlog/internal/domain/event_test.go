package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestQuarantineIdentityIsStableAndFallbackDerived(t *testing.T) {
	t.Parallel()
	env := map[string]any{"id": "evt-1", "type": "zone.temp", "data": map[string]any{"celsius": 30}}
	first, err := NewQuarantinePayload(env)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewQuarantinePayload(env)
	if err != nil {
		t.Fatal(err)
	}
	if first.QuarantineID != second.QuarantineID || first.EventID != "evt-1" || first.EventType != "zone.temp" {
		t.Fatalf("identity is not stable: %+v vs %+v", first, second)
	}
	if !strings.HasPrefix(first.QuarantineID, "q_") {
		t.Fatalf("quarantine id = %q", first.QuarantineID)
	}
	if first.OverflowGapID() != first.QuarantineID+":gap" {
		t.Fatalf("gap id = %q", first.OverflowGapID())
	}

	anonymous, err := NewQuarantinePayload(map[string]any{"data": 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(anonymous.EventID, "payload:") {
		t.Fatalf("fallback id = %q", anonymous.EventID)
	}
}

func TestQuarantineConflictFollowsPayloadBytes(t *testing.T) {
	t.Parallel()
	base, err := NewQuarantinePayload(map[string]any{"id": "evt-1", "v": 1})
	if err != nil {
		t.Fatal(err)
	}
	if base.ConflictingPayload(base.Digest) {
		t.Fatal("identical payload reported as conflict")
	}
	other, err := NewQuarantinePayload(map[string]any{"id": "evt-1", "v": 2})
	if err != nil {
		t.Fatal(err)
	}
	if !base.ConflictingPayload(other.Digest) {
		t.Fatal("different payload under same id not detected")
	}
}

func TestQuarantineMarshalFailureIsWrapped(t *testing.T) {
	t.Parallel()
	_, err := NewQuarantinePayload(map[string]any{"bad": make(chan int)})
	if err == nil || !strings.Contains(err.Error(), "marshal quarantined payload") {
		t.Fatalf("err = %v", err)
	}
}

func TestUseCaseInputsFailClosed(t *testing.T) {
	t.Parallel()
	if err := ValidQuarantine("", "reason", "now"); err == nil {
		t.Fatal("tenantless quarantine accepted")
	}
	if err := ValidRelease("tenant", "", "now"); err == nil {
		t.Fatal("eventless release accepted")
	}
}

func TestEncodeEventBodyDigestsPayloadBytes(t *testing.T) {
	t.Parallel()
	encoded, err := EncodeEventBody(map[string]any{"celsius": 30.5}, map[string]any{"grade": "A"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := EncodeEventBody(map[string]any{"celsius": 30.5}, map[string]any{"grade": "A"})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded.PayloadJSON) != string(again.PayloadJSON) || len(encoded.PayloadSHA256) != 32 {
		t.Fatalf("encoding is not stable: %+v", encoded)
	}
	var quality map[string]any
	if err := json.Unmarshal(encoded.QualityJSON, &quality); err != nil || quality["grade"] != "A" {
		t.Fatalf("quality encoding = %s err=%v", encoded.QualityJSON, err)
	}
	if _, err := EncodeEventBody(map[string]any{"bad": make(chan int)}, nil); err == nil || !strings.Contains(err.Error(), "marshal payload") {
		t.Fatalf("err = %v", err)
	}
	if _, err := EncodeEventBody(nil, make(chan int)); err == nil {
		t.Fatal("unmarshalable quality accepted")
	}
}

func TestStoredTimesParseInColumnOrder(t *testing.T) {
	t.Parallel()
	eventTime := "2026-01-01T00:00:00Z"
	observed := "2026-01-01T00:00:01Z"
	parsedEvent, parsedIngested, parsedObserved, err := StoredTimes{EventTime: eventTime, IngestedAt: eventTime, ObservedAt: &observed}.Parse()
	if err != nil || parsedObserved == nil || !parsedEvent.Equal(parsedIngested) {
		t.Fatalf("parse = (%v, %v, %v) err=%v", parsedEvent, parsedIngested, parsedObserved, err)
	}
	if _, _, _, err := (StoredTimes{EventTime: "bad"}).Parse(); err == nil || !strings.Contains(err.Error(), "parse event_time") {
		t.Fatalf("err = %v", err)
	}
	if _, _, _, err := (StoredTimes{EventTime: eventTime, IngestedAt: "bad"}).Parse(); err == nil || !strings.Contains(err.Error(), "parse ingested_at") {
		t.Fatalf("err = %v", err)
	}
	badObserved := "bad"
	if _, _, _, err := (StoredTimes{EventTime: eventTime, IngestedAt: eventTime, ObservedAt: &badObserved}).Parse(); err == nil || !strings.Contains(err.Error(), "parse observed_at") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadLimitDefaultsToThousand(t *testing.T) {
	t.Parallel()
	for limit, want := range map[int]int{-5: 1000, 0: 1000, 7: 7, 2000: 2000} {
		if got := ReadLimit(limit); got != want {
			t.Fatalf("ReadLimit(%d) = %d want %d", limit, got, want)
		}
	}
}
