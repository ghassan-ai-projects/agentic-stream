package domain

import (
	"errors"
	"testing"
)

func TestSocketPathMustBeCleanAbsoluteAndLocal(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"/tmp/live.sock", "/run/agentic/live.sock"} {
		if err := ValidateSocketPath(ok); err != nil {
			t.Fatalf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "live.sock", "/tmp/../live.sock", "/tmp//live.sock", "unix:///tmp/live.sock", "/tmp/live\x00.sock"} {
		if err := ValidateSocketPath(bad); !errors.Is(err, errSocketPath) {
			t.Fatalf("%q: err = %v, want the unsafe path refusal", bad, err)
		}
	}
}
