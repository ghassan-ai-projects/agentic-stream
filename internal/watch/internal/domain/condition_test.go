package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/actionport"
)

var testNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func installCommand(mutate func(map[string]any)) actionport.Command {
	payload := map[string]any{"expression": "features.temperature > 90", "target": "motor-1", "situation_id": "sit-1",
		"situation_version": 1, "max_fires": 3, "expires_at": testNow.Add(time.Hour).Format(time.RFC3339Nano)}
	if mutate != nil {
		mutate(payload)
	}
	return actionport.Command{CommandID: "cmd-1", TenantID: "tenant", EffectorRoute: InstallRoute, Payload: payload}
}

func TestConditionFromCommandAcceptsAValidPayload(t *testing.T) {
	t.Parallel()
	condition, err := ConditionFromCommand(installCommand(nil), testNow)
	if err != nil {
		t.Fatal(err)
	}
	want := Condition{TenantID: "tenant", SituationID: "sit-1", Expression: "features.temperature > 90", Target: "motor-1",
		ExpiresAt: testNow.Add(time.Hour).Format(time.RFC3339Nano), SituationVersion: 1, MaxFires: 3}
	if condition != want {
		t.Fatalf("condition = %+v, want %+v", condition, want)
	}
}

func TestConditionFromCommandRefusesInvalidPayloadsInPrecedenceOrder(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"missing expression", func(p map[string]any) { delete(p, "expression") }, "watch condition payload is invalid"},
		{"oversize expression", func(p map[string]any) { p["expression"] = strings.Repeat("a", 4097) }, "watch condition payload is invalid"},
		{"missing target", func(p map[string]any) { delete(p, "target") }, "watch condition payload is invalid"},
		{"missing Situation", func(p map[string]any) { delete(p, "situation_id") }, "watch condition payload is invalid"},
		{"non-numeric version", func(p map[string]any) { p["situation_version"] = "1" }, "watch condition payload is invalid"},
		{"zero version", func(p map[string]any) { p["situation_version"] = 0 }, "watch condition payload is invalid"},
		{"zero fires", func(p map[string]any) { p["max_fires"] = 0 }, "watch condition payload is invalid"},
		{"too many fires", func(p map[string]any) { p["max_fires"] = 101 }, "watch condition payload is invalid"},
		{"identity precedes expression", func(p map[string]any) { p["max_fires"] = 0; p["expression"] = "features.x;" }, "watch condition payload is invalid"},
		{"expression precedes expiry", func(p map[string]any) { p["expression"] = "features.x;"; p["expires_at"] = "invalid" }, "forbidden syntax"},
		{"unparseable expiry", func(p map[string]any) { p["expires_at"] = "soon" }, "watch condition expiry is invalid"},
		{"expiry not in the future", func(p map[string]any) { p["expires_at"] = testNow.Format(time.RFC3339Nano) }, "watch condition expiry is invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ConditionFromCommand(installCommand(tc.mutate), testNow)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestIntegerPayloadAcceptsWholeNumbersOnly(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		value any
		want  int
		ok    bool
	}{
		{"int", 4, 4, true}, {"int64", int64(5), 5, true}, {"whole float", float64(6), 6, true},
		{"fractional float", 6.5, 6, false}, {"json number", json.Number("7"), 7, true},
		{"bad json number", json.Number("x"), 0, false}, {"string", "8", 0, false}, {"nil", nil, 0, false},
	}
	for _, tc := range cases {
		if got, ok := integerPayload(tc.value); ok != tc.ok || (ok && got != tc.want) {
			t.Fatalf("%s: got %d,%v want %d,%v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestInstallIsIdempotentOnlyForTheSameCondition(t *testing.T) {
	t.Parallel()
	condition, err := ConditionFromCommand(installCommand(nil), testNow)
	if err != nil || condition.SameAs(condition) != nil {
		t.Fatalf("identical condition refused: %v", err)
	}
	changed := condition
	changed.MaxFires++
	if err := condition.SameAs(changed); err == nil || err.Error() != "watch command idempotency conflict" {
		t.Fatalf("conflict err = %v", err)
	}
}

func TestWatchIdentityPrefersTheIdempotencyKey(t *testing.T) {
	t.Parallel()
	if WatchID(actionport.Command{CommandID: "cmd"}) != "cmd" || WatchID(actionport.Command{CommandID: "cmd", IdempotencyKey: "key"}) != "key" {
		t.Fatal("watch identity changed")
	}
}

func TestActiveWatchScopeAndRoute(t *testing.T) {
	t.Parallel()
	active := ActiveWatch{SituationID: "sit-1", Target: "motor-1"}
	if !active.InScope("sit-1", "motor-1") || active.InScope("sit-2", "motor-1") || active.InScope("sit-1", "motor-2") {
		t.Fatal("watch scope changed")
	}
	if err := RequireRoute(actionport.Command{EffectorRoute: InstallRoute}); err != nil {
		t.Fatal(err)
	}
	if err := RequireRoute(actionport.Command{EffectorRoute: "other"}); err == nil || !strings.Contains(err.Error(), `"other"`) {
		t.Fatalf("route err = %v", err)
	}
}
