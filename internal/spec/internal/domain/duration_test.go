package domain

import (
	"strings"
	"testing"
	"time"
)

func TestParseDurationAcceptsGoUnitsAndWholeDays(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    time.Duration
		wantErr string
	}{
		{in: "15m", want: 15 * time.Minute},
		{in: "6h", want: 6 * time.Hour},
		{in: "1h30m", want: 90 * time.Minute},
		{in: "30d", want: 30 * 24 * time.Hour},
		{in: "1d", want: 24 * time.Hour},
		{in: "106751d", want: 106751 * 24 * time.Hour},
		{in: "", wantErr: "empty duration"},
		{in: "d", wantErr: "parse days"},
		{in: "1.5d", wantErr: "parse days"},
		{in: "0d", wantErr: "must be positive"},
		{in: "-2d", wantErr: "must be positive"},
		{in: "106752d", wantErr: "overflows time.Duration"},
		{in: "soon", wantErr: "parse duration"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := ParseDuration(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseDuration(%q) error = %v, want %q", tt.in, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("ParseDuration(%q) = %v, %v, want %v", tt.in, got, err, tt.want)
			}
		})
	}
}
