package domain

import (
	"errors"
	"testing"
)

func TestValidateChangeRequiresKnownStatusReasonVersionAndTime(t *testing.T) {
	t.Parallel()
	for name, args := range map[string][4]any{
		"status":  {"paused", "r", int64(1), "now"},
		"reason":  {StatusReady, "", int64(1), "now"},
		"version": {StatusReady, "r", int64(0), "now"},
		"time":    {StatusTripped, "r", int64(1), ""},
	} {
		if err := ValidateChange(args[0].(string), args[1].(string), args[2].(int64), args[3].(string)); err == nil {
			t.Errorf("%s: invalid change accepted", name)
		}
	}
	if err := ValidateChange(StatusTripped, "stop", 2, "now"); err != nil {
		t.Fatalf("valid change: %v", err)
	}
}

func TestRequireReadyFailsClosedWithTheStoredReason(t *testing.T) {
	t.Parallel()
	if err := RequireReady(StatusReady, ""); err != nil {
		t.Fatalf("ready: %v", err)
	}
	err := RequireReady(StatusTripped, "maintenance")
	if !errors.Is(err, ErrTripped) || err.Error() != "runtime interlock is tripped: maintenance" {
		t.Fatalf("tripped = %v", err)
	}
}
