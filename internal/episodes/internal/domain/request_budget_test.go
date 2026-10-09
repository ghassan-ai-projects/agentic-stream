package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestWallTimeBudgetValidatesOnceAtRequestBoundary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		raw       string
		want      int64
		wantErr   bool
		validated bool
	}{
		{name: "missing", raw: `{}`, want: 0, validated: true},
		{name: "valid", raw: `{"budget":{"wall_time":"250ms"}}`, want: 250000000, validated: true},
		{name: "schema day unit", raw: `{"budget":{"wall_time":"1d"}}`, want: 24 * 60 * 60 * 1_000_000_000, validated: true},
		{name: "invalid duration", raw: `{"budget":{"wall_time":"soon"}}`, wantErr: true},
		{name: "zero", raw: `{"budget":{"wall_time":"0s"}}`, wantErr: true},
		{name: "negative", raw: `{"budget":{"wall_time":"-1s"}}`, wantErr: true},
		{name: "invalid json", raw: `{`, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			req := &Request{RequestJSON: []byte(test.raw)}
			got, err := req.WallTimeBudget()
			if (err != nil) != test.wantErr {
				t.Fatalf("WallTimeBudget error=%v, wantErr=%v", err, test.wantErr)
			}
			if err == nil && got.Nanoseconds() != test.want {
				t.Fatalf("WallTimeBudget=%s, want %dns", got, test.want)
			}
			if req.wallTimeValidated != test.validated {
				t.Fatalf("wallTimeValidated=%v, want %v", req.wallTimeValidated, test.validated)
			}
			if err == nil {
				again, againErr := req.WallTimeBudget()
				if againErr != nil || again != got {
					t.Fatalf("second WallTimeBudget=%s err=%v, want %s", again, againErr, got)
				}
			}
		})
	}
}

func requestWithBudget(wallTime string) []byte {
	document := map[string]any{"budget": map[string]any{"wall_time": wallTime}}
	raw, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}
	return raw
}

func TestParseWallTimeBudgetValidatesBounds(t *testing.T) {
	t.Parallel()
	if duration, err := ParseWallTimeBudget(requestWithBudget("90s")); err != nil || duration != 90*time.Second {
		t.Fatalf("budget = %v err = %v", duration, err)
	}
	if duration, err := ParseWallTimeBudget(requestWithBudget("")); err != nil || duration != 0 {
		t.Fatalf("empty budget = %v err = %v", duration, err)
	}
	for name, tc := range map[string]struct{ wallTime, want string }{
		"zero":        {"0s", "invalid wall_time budget"},
		"negative":    {"-5s", "invalid wall_time budget"},
		"unparseable": {"soon", "invalid wall_time budget"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseWallTimeBudget(requestWithBudget(tc.wallTime)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want one containing %q", err, tc.want)
			}
		})
	}
	if _, err := ParseWallTimeBudget([]byte("not-json")); err == nil || !strings.Contains(err.Error(), "decode episode budget") {
		t.Fatalf("err = %v", err)
	}
}
