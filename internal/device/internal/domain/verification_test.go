package domain

import (
	"strings"
	"testing"
)

func TestOutputVerified(t *testing.T) {
	t.Parallel()
	command := map[string]any{"target": "fan-01", "operation": "set_pwm", "parameters": map[string]any{"duty_permille": float64(400), "lease_ms": float64(5000)}}
	output := func(value float64, energized bool) map[string]any {
		return map[string]any{"current_output": map[string]any{"target": "fan-01", "operation": "set_pwm", "value": value, "energized": energized}}
	}
	for _, tt := range []struct {
		name  string
		state map[string]any
		want  bool
	}{
		{"matching duty", output(400, true), true},
		{"energized at the wrong duty", output(300, true), false},
		{"right duty but not energized", output(400, false), false},
		{"other target", map[string]any{"current_output": map[string]any{"target": "led-01", "operation": "set_pwm", "value": float64(400), "energized": true}}, false},
	} {
		if got, err := OutputVerified(tt.state, command); err != nil || got != tt.want {
			t.Errorf("%s: OutputVerified = %v, %v; want %v", tt.name, got, err, tt.want)
		}
	}
	off := map[string]any{"target": "fan-01", "operation": "set_pwm", "parameters": map[string]any{"duty_permille": float64(0)}}
	if got, err := OutputVerified(output(0, false), off); err != nil || !got {
		t.Fatalf("de-energized output = %v, %v", got, err)
	}
	ambiguous := map[string]any{"parameters": map[string]any{"a": float64(1), "b": float64(2)}}
	if _, err := OutputVerified(output(1, true), ambiguous); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("ambiguous output = %v", err)
	}
}
