package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEvidenceQueryNarrowsButNeverWidensTheScope(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	scope := EvidenceScope{EntityID: "motor-1", MaxRows: 10, MaxBytes: 1024, Window: 6 * time.Hour}
	tests := []struct {
		name      string
		args      string
		wantRows  uint64
		wantBytes uint64
		wantFrom  time.Time
		wantUntil time.Time
	}{
		{"defaults to the scope window before now", `{}`, 10, 1024, now.Add(-6 * time.Hour), now},
		{"narrower rows and bytes", `{"max_rows":3,"max_bytes":100}`, 3, 100, now.Add(-6 * time.Hour), now},
		{"wider rows and bytes are ignored", `{"max_rows":99,"max_bytes":99999}`, 10, 1024, now.Add(-6 * time.Hour), now},
		{"the scope's own entity", `{"entity_id":"motor-1"}`, 10, 1024, now.Add(-6 * time.Hour), now},
		{"explicit window inside the scope", `{"from":"2026-08-12T07:00:00Z","until":"2026-08-12T08:00:00Z"}`, 10, 1024,
			time.Date(2026, 8, 12, 7, 0, 0, 0, time.UTC), time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)},
		{"a window past the horizon is cut at it", `{"from":"2026-08-12T11:00:00Z","until":"2026-08-12T13:00:00Z"}`, 10, 1024,
			time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC), now},
		{"a window before the scope starts at it", `{"from":"2026-08-12T01:00:00Z","until":"2026-08-12T07:00:00Z"}`, 10, 1024,
			now.Add(-6 * time.Hour), time.Date(2026, 8, 12, 7, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			query, err := scope.Query(json.RawMessage(tt.args), now)
			if err != nil {
				t.Fatalf("Query() = %v", err)
			}
			if query.MaxRows != tt.wantRows || query.MaxBytes != tt.wantBytes || !query.From.Equal(tt.wantFrom) || !query.Until.Equal(tt.wantUntil) {
				t.Fatalf("Query() = %+v, want rows %d bytes %d from %v until %v", query, tt.wantRows, tt.wantBytes, tt.wantFrom, tt.wantUntil)
			}
		})
	}
}

func TestEvidenceQueryRejectsArgumentsOutsideTheScope(t *testing.T) {
	t.Parallel()
	scope := EvidenceScope{EntityID: "motor-1", MaxRows: 10, MaxBytes: 1024, Window: time.Hour}
	tests := []struct {
		name string
		args string
		want string
	}{
		{"another entity", `{"entity_id":"motor-2"}`, "outside episode scope"},
		{"malformed from", `{"from":"yesterday"}`, "invalid evidence from"},
		{"malformed until", `{"until":"tomorrow"}`, "invalid evidence until"},
		{"until before from", `{"from":"2026-08-12T12:00:00Z","until":"2026-08-12T11:00:00Z"}`, "until must be after from"},
		{"empty window", `{"from":"2026-08-12T12:00:00Z","until":"2026-08-12T12:00:00Z"}`, "until must be after from"},
		{"a window wholly before the scope", `{"from":"2026-08-12T01:00:00Z","until":"2026-08-12T02:00:00Z"}`, "until must be after from"},
		{"arguments that are not json", `not json`, "decode evidence arguments"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if query, err := scope.Query(json.RawMessage(tt.args), time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Query() = %+v, %v; want error containing %q", query, err, tt.want)
			}
		})
	}
}
