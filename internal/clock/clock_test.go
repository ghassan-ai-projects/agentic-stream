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

func TestVirtualAdvanceFiresDueTimersBehindLaterHead(t *testing.T) {
	// Regression: timers scheduled out of due order must all fire during the
	// Advance in which they become due, even when a not-yet-due timer was
	// scheduled first and would otherwise sit at the head of the queue.
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	v := clock.NewVirtual(start)

	t10 := v.NewTimer(10 * time.Minute)
	t5 := v.NewTimer(5 * time.Minute)
	t7 := v.NewTimer(7 * time.Minute)

	v.Advance(7 * time.Minute)

	fired := func(name string, timer clock.Timer) bool {
		select {
		case f := <-timer.C():
			_ = f
			return true
		default:
			return false
		}
	}

	if !fired("t5", t5) {
		t.Error("timer due at +5m did not fire during Advance(7m)")
	}
	if !fired("t7", t7) {
		t.Error("timer due at +7m did not fire during Advance(7m)")
	}
	if fired("t10", t10) {
		t.Error("timer due at +10m fired early during Advance(7m)")
	}

	// The +10m timer fires on a later advance.
	v.Advance(3 * time.Minute)
	if !fired("t10", t10) {
		t.Error("timer due at +10m did not fire during Advance to +10m")
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
