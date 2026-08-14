package notifycontract

import (
	"strings"
	"testing"
)

func TestGoldenEventsConform(t *testing.T) {
	events, err := GoldenEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != len(Types()) {
		t.Fatalf("golden event count = %d, want %d", len(events), len(Types()))
	}
	for _, event := range events {
		t.Run(event.Type, func(t *testing.T) {
			digest, err := event.ComputeEnvelopeDigest()
			if err != nil {
				t.Fatal(err)
			}
			if event.EnvelopeDigest != digest {
				t.Fatalf("envelope digest = %q, want %q", event.EnvelopeDigest, digest)
			}
			if err := Validate(event); err != nil {
				t.Fatalf("golden event does not conform: %v", err)
			}
		})
	}
}

func TestValidateRejectsUnknownVersionAndAuthorityMismatch(t *testing.T) {
	events, err := GoldenEvents()
	if err != nil {
		t.Fatal(err)
	}
	unknown := events[0]
	unknown.Type = strings.TrimSuffix(unknown.Type, ".v1") + ".v2"
	if err := Validate(unknown); err == nil {
		t.Fatal("unknown notification version was accepted")
	}

	unauthorized := events[0]
	data := unauthorized.Data.(map[string]any)
	data["source_authority"] = "//agentic-stream/tenant/other"
	digest, err := unauthorized.ComputeEnvelopeDigest()
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.EnvelopeDigest = digest
	if err := Validate(unauthorized); err == nil {
		t.Fatal("authority mismatch was accepted")
	}
}
