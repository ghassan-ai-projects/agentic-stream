package duration

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{in: "15m", want: 15 * time.Minute},
		{in: "6h", want: 6 * time.Hour},
		{in: "1h30m", want: 90 * time.Minute},
		{in: "30d", want: 30 * 24 * time.Hour},
		{in: "1d", want: 24 * time.Hour},
		{in: "", wantErr: true},
		{in: "d", wantErr: true},
		{in: "1.5d", wantErr: true},
		{in: "0d", wantErr: true},
		{in: "-2d", wantErr: true},
		{in: "106752d", wantErr: true}, // exceeds time.Duration's ~292 years
		{in: "soon", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			got, err := Parse(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("Parse(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
