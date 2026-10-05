package domain

import "testing"

func TestWallTimeBudgetValidatesOnceAtRequestBoundary(t *testing.T) {
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
