package wire

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence/internal/domain"
)

func TestCallFingerprintEncodingIsTheStoredV1Contract(t *testing.T) {
	t.Parallel()
	const document = `{"episode_id":"episode-1","call_id":"call-1","tool_name":"evidence.get","tenant_id":"tenant-1","situation_id":"situation-1","entity_id":"motor-1","attempt_id":"attempt-1","situation_version":1,"fence":1,"arguments":{"entity_id":"motor-1"},"deadline":"2026-08-12T12:01:00.000000000Z","from":"2026-08-12T11:00:00.000000000Z","until":"2026-08-12T12:00:00.000000000Z","max_rows":1,"max_bytes":100,"traceparent":"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01","tracestate":""}`
	want := sha256.Sum256([]byte(document))
	got, err := CallFingerprint(fingerprintCall())
	if err != nil || hex.EncodeToString(got) != hex.EncodeToString(want[:]) {
		t.Fatalf("fingerprint = %x, err = %v, want %x", got, err, want)
	}
}

func TestCallFingerprintChangesWithEveryRequestDimension(t *testing.T) {
	t.Parallel()
	original, err := CallFingerprint(fingerprintCall())
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*domain.Call){
		"episode":           func(c *domain.Call) { c.EpisodeID = "other" },
		"call":              func(c *domain.Call) { c.CallID = "other" },
		"tool":              func(c *domain.Call) { c.ToolName = "other" },
		"tenant":            func(c *domain.Call) { c.TenantID = "other" },
		"situation":         func(c *domain.Call) { c.SituationID = "other" },
		"entity":            func(c *domain.Call) { c.EntityID = "other" },
		"attempt":           func(c *domain.Call) { c.AttemptID = "other" },
		"situation version": func(c *domain.Call) { c.SituationVersion++ },
		"fence":             func(c *domain.Call) { c.Fence++ },
		"arguments":         func(c *domain.Call) { c.Arguments.EntityID = "other" },
		"deadline":          func(c *domain.Call) { c.Deadline = c.Deadline.Add(time.Nanosecond) },
		"range start":       func(c *domain.Call) { c.From = c.From.Add(time.Nanosecond) },
		"range end":         func(c *domain.Call) { c.Until = c.Until.Add(time.Nanosecond) },
		"row budget":        func(c *domain.Call) { c.MaxRows++ },
		"byte budget":       func(c *domain.Call) { c.MaxBytes++ },
		"trace":             func(c *domain.Call) { c.Trace.Traceparent = "other" },
		"trace state":       func(c *domain.Call) { c.Trace.Tracestate = "vendor=1" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			call := fingerprintCall()
			mutate(&call)
			changed, err := CallFingerprint(call)
			if err != nil || string(changed) == string(original) {
				t.Fatalf("fingerprint did not change with the %s (err %v)", name, err)
			}
		})
	}
}
