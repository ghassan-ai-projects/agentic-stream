package clock_test

import (
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
)

func TestVirtualClockAdvances(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	v := clock.NewVirtual(start)
	if got := v.Now(); !got.Equal(start) {
		t.Fatalf("Now() = %v, want %v", got, start)
	}
	v.Advance(time.Hour)
	if got := v.Now(); !got.Equal(start.Add(time.Hour)) {
		t.Fatalf("Now() = %v, want %v", got, start.Add(time.Hour))
	}
}

func TestVirtualTimerFires(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	v := clock.NewVirtual(start)
	timer := v.NewTimer(time.Minute)

	select {
	case <-timer.C():
		t.Fatal("timer fired before Advance")
	default:
	}

	v.Advance(time.Minute)
	select {
	case fired := <-timer.C():
		if !fired.Equal(start.Add(time.Minute)) {
			t.Fatalf("fired at %v, want %v", fired, start.Add(time.Minute))
		}
	default:
		t.Fatal("timer did not fire")
	}
}

func TestVirtualTimerStop(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	v := clock.NewVirtual(start)
	timer := v.NewTimer(time.Minute)
	if !timer.Stop() {
		t.Fatal("Stop returned false for pending timer")
	}
	v.Advance(time.Minute)
	select {
	case <-timer.C():
		t.Fatal("stopped timer fired")
	default:
	}
}
