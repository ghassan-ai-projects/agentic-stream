package domain

import (
	"testing"
	"time"
)

func TestTheCallTimeoutIsNineTenthsOfTheLease(t *testing.T) {
	t.Parallel()
	cases := []struct{ lease, want time.Duration }{
		{time.Minute, 54 * time.Second},
		{3 * time.Minute, 162 * time.Second},
		{time.Nanosecond, time.Nanosecond},
	}
	for _, tc := range cases {
		if got := CallTimeout(tc.lease); got != tc.want {
			t.Errorf("CallTimeout(%s) = %s, want %s", tc.lease, got, tc.want)
		}
	}
}
