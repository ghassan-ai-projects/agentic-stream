package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateChangeRequiresKnownStatusReasonVersionAndTime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		status  string
		reason  string
		version int64
		now     string
		want    string
	}{
		{"tripped", StatusTripped, "stop", 2, "now", ""},
		{"ready", StatusReady, "inspected", 1, "now", ""},
		{"unknown status", "paused", "r", 1, "now", `invalid interlock status "paused"`},
		{"empty status", "", "r", 1, "now", `invalid interlock status ""`},
		{"missing reason", StatusReady, "", 1, "now", "reason, version, and timestamp are required"},
		{"version zero", StatusReady, "r", 0, "now", "reason, version, and timestamp are required"},
		{"negative version", StatusReady, "r", -1, "now", "reason, version, and timestamp are required"},
		{"missing time", StatusTripped, "r", 1, "", "reason, version, and timestamp are required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateChange(tt.status, tt.reason, tt.version, tt.now)
			if tt.want == "" && err != nil || tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("ValidateChange = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestNextStateAdvancesTheVersionAndRecordsTheChange(t *testing.T) {
	t.Parallel()
	current := State{Status: StatusReady, Reason: "seeded", UpdatedAt: "t0", Version: 4}
	got, err := NextState(current, StatusTripped, "operator stop", "t1")
	want := State{Status: StatusTripped, Reason: "operator stop", UpdatedAt: "t1", Version: 5}
	if err != nil || got != want {
		t.Fatalf("NextState = %+v, %v; want %+v", got, err, want)
	}
}

func TestNextStateRefusesAnInvalidChangeAndReturnsNoState(t *testing.T) {
	t.Parallel()
	current := State{Status: StatusReady, Reason: "seeded", UpdatedAt: "t0", Version: 1}
	tests := []struct {
		name                string
		status, reason, now string
		want                string
	}{
		{"unknown status", "paused", "r", "t1", "invalid interlock status"},
		{"missing reason", StatusTripped, "", "t1", "required"},
		{"missing time", StatusTripped, "r", "", "required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NextState(current, tt.status, tt.reason, tt.now)
			if err == nil || !strings.Contains(err.Error(), tt.want) || got != (State{}) {
				t.Fatalf("NextState = %+v, %v; want the zero State and an error containing %q", got, err, tt.want)
			}
		})
	}
}

func TestRequireReadyFailsClosedWithTheStoredReason(t *testing.T) {
	t.Parallel()
	if err := RequireReady(StatusReady, ""); err != nil {
		t.Fatalf("ready: %v", err)
	}
	for _, status := range []string{StatusTripped, "", "unknown"} {
		err := RequireReady(status, "maintenance")
		if !errors.Is(err, ErrTripped) || err.Error() != "runtime interlock is tripped: maintenance" {
			t.Fatalf("RequireReady(%q) = %v, want ErrTripped with the stored reason", status, err)
		}
	}
}
