package transport

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/sources/internal/domain"
)

func TestRandomIsPrefixedAndUnique(t *testing.T) {
	t.Parallel()

	generator := Random()
	seen := map[string]bool{}
	for range 1000 {
		id := generator.New(domain.PrefixEvent)
		if !strings.HasPrefix(id, domain.PrefixEvent) || len(id) != len(domain.PrefixEvent)+16 {
			t.Fatalf("id %q is not a %s-prefixed 12-byte base64url value", id, domain.PrefixEvent)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestPhysicalClockIsUTCAndItsTimersFire(t *testing.T) {
	t.Parallel()
	clock := Physical()
	if clock.Now().Location() != time.UTC {
		t.Fatal("physical clock must report UTC")
	}
	timer := clock.NewTimer(time.Millisecond)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	select {
	case <-timer.C():
	case <-ctx.Done():
		t.Fatal("timer did not fire")
	}
	if timer.Stop() {
		t.Fatal("a fired timer cannot be stopped")
	}
}

func TestPhysicalTimerStopPreventsFiring(t *testing.T) {
	t.Parallel()
	timer := Physical().NewTimer(time.Hour)
	if !timer.Stop() {
		t.Fatal("Stop returned false for a pending timer")
	}
	select {
	case <-timer.C():
		t.Fatal("stopped timer fired")
	default:
	}
}
