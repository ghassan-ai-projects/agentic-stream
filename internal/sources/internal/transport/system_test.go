package transport

import (
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

func TestPhysicalClockIsUTCAndTimersFire(t *testing.T) {
	t.Parallel()
	clock := Physical()
	if clock.Now().Location() != time.UTC {
		t.Fatal("physical clock must report UTC")
	}
	timer := clock.NewTimer(time.Millisecond)
	select {
	case <-timer.C():
	case <-time.After(time.Second):
		t.Fatal("timer did not fire")
	}
	if timer.Stop() {
		t.Fatal("a fired timer cannot be stopped")
	}
}
