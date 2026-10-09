package domain

import (
	"strings"
	"testing"
	"time"
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
		t.Fatalf("quarantine id = %q, want the q_ prefix", first.QuarantineID)
	}
	if first.OverflowGapID() != first.QuarantineID+":gap" {
		t.Fatalf("gap id = %q, want %q", first.OverflowGapID(), first.QuarantineID+":gap")
	}

	anonymous, err := NewQuarantinePayload(map[string]any{"data": 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(anonymous.EventID, "payload:") {
		t.Fatalf("fallback id = %q, want the payload: prefix", anonymous.EventID)
	}
}

func TestQuarantineReadsItsHeaderFromTheEnvelopeFields(t *testing.T) {
	t.Parallel()
	payload, err := NewQuarantinePayload(map[string]any{"id": "evt-1", "type": "zone.temp", "schema_version": "1.0", "source": "gateway"})
	if err != nil {
		t.Fatal(err)
	}
	if payload.EventID != "evt-1" || payload.EventType != "zone.temp" || payload.SchemaVersion != "1.0" || payload.Source != "gateway" {
		t.Fatalf("header = %+v, want id evt-1, type zone.temp, schema 1.0, source gateway", payload)
	}
}

func TestQuarantineIdentityFollowsPayloadBytes(t *testing.T) {
	t.Parallel()
	base, err := NewQuarantinePayload(map[string]any{"id": "evt-1", "v": 1})
	if err != nil {
		t.Fatal(err)
	}
	same, err := NewQuarantinePayload(map[string]any{"v": 1, "id": "evt-1"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewQuarantinePayload(map[string]any{"id": "evt-1", "v": 2})
	if err != nil {
		t.Fatal(err)
	}
	if base.QuarantineID != same.QuarantineID || base.ConflictingPayload(same.Digest) {
		t.Fatal("the same payload with reordered keys must keep one identity and not conflict")
	}
	if base.QuarantineID == other.QuarantineID {
		t.Fatalf("different payloads under one event id share quarantine id %s", base.QuarantineID)
	}
	if !base.ConflictingPayload(other.Digest) {
		t.Fatal("different payload under the same event id was not reported as a conflict")
	}
}

func TestQuarantineMarshalFailureIsWrapped(t *testing.T) {
	t.Parallel()
	_, err := NewQuarantinePayload(map[string]any{"bad": make(chan int)})
	if err == nil || !strings.Contains(err.Error(), "marshal quarantined payload") {
		t.Fatalf("err = %v, want marshal quarantined payload", err)
	}
}

func TestQuarantineAndReleaseRequireTenantEventAndTime(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"quarantine without tenant", ValidQuarantine("", "reason", now), "tenant, reason, and time are required"},
		{"quarantine without reason", ValidQuarantine("tenant", "", now), "tenant, reason, and time are required"},
		{"quarantine without time", ValidQuarantine("tenant", "reason", time.Time{}), "tenant, reason, and time are required"},
		{"release without tenant", ValidRelease("", "evt", now), "tenant, event, and time are required"},
		{"release without event", ValidRelease("tenant", "", now), "tenant, event, and time are required"},
		{"release without time", ValidRelease("tenant", "evt", time.Time{}), "tenant, event, and time are required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.err == nil || tc.err.Error() != tc.want {
				t.Fatalf("err = %v, want %q", tc.err, tc.want)
			}
		})
	}
	if err := ValidQuarantine("tenant", "reason", now); err != nil {
		t.Fatalf("complete quarantine refused: %v", err)
	}
	if err := ValidRelease("tenant", "evt", now); err != nil {
		t.Fatalf("complete release refused: %v", err)
	}
}

func TestOperatorStatusShowsRedrivenOnlyForReleasedRecords(t *testing.T) {
	t.Parallel()
	cases := []struct {
		stored   string
		redriven bool
		want     string
	}{
		{"released", true, "redriven"},
		{"released", false, "released"},
		{"quarantined", false, "quarantined"},
		{"rejected", true, "rejected"},
	}
	for _, tc := range cases {
		if got := OperatorStatus(tc.stored, tc.redriven); got != tc.want {
			t.Errorf("OperatorStatus(%q, %v) = %q, want %q", tc.stored, tc.redriven, got, tc.want)
		}
	}
}
