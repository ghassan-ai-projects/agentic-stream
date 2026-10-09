package domain

import (
	"testing"
	"time"
)

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func fired(timer Timer) (time.Time, bool) {
	select {
	case at := <-timer.C():
		return at, true
	default:
		return time.Time{}, false
	}
}

func TestVirtualClockStartsAtItsStartInUTCAndAdvances(t *testing.T) {
	t.Parallel()
	zone := time.FixedZone("plus2", 2*3600)
	v := NewVirtual(epoch.In(zone))
	if got := v.Now(); !got.Equal(epoch) || got.Location() != time.UTC {
		t.Fatalf("Now() = %v, want %v in UTC", got, epoch)
	}
	v.Advance(time.Hour)
	if got, want := v.Now(), epoch.Add(time.Hour); !got.Equal(want) {
		t.Fatalf("Now() after Advance = %v, want %v", got, want)
	}
}

func TestQualityNamesTheClockKind(t *testing.T) {
	t.Parallel()
	if got := Quality(NewVirtual(epoch)); got != "virtual" {
		t.Fatalf("Quality(virtual) = %q", got)
	}
	if got := Quality(otherClock{}); got != "physical" {
		t.Fatalf("Quality(any other clock) = %q, want physical", got)
	}
}

type otherClock struct{ Clock }

func TestVirtualTimerFiresOnlyOnceTheClockReachesItsDueTime(t *testing.T) {
	t.Parallel()
	v := NewVirtual(epoch)
	timer := v.NewTimer(time.Minute)

	if _, ok := fired(timer); ok {
		t.Fatal("timer fired before Advance")
	}
	v.Advance(time.Minute - time.Nanosecond)
	if _, ok := fired(timer); ok {
		t.Fatal("timer fired one nanosecond before it was due")
	}
	v.Advance(time.Nanosecond)
	at, ok := fired(timer)
	if !ok || !at.Equal(epoch.Add(time.Minute)) {
		t.Fatalf("timer fired = %v at %v, want it to fire with its due time %v", ok, at, epoch.Add(time.Minute))
	}
}

func TestVirtualAdvanceFiresDueTimersBehindLaterHead(t *testing.T) {
	t.Parallel()
	v := NewVirtual(epoch)
	t10 := v.NewTimer(10 * time.Minute)
	t5 := v.NewTimer(5 * time.Minute)
	t7 := v.NewTimer(7 * time.Minute)

	v.Advance(7 * time.Minute)

	if _, ok := fired(t5); !ok {
		t.Error("timer due at +5m did not fire during Advance(7m)")
	}
	if _, ok := fired(t7); !ok {
		t.Error("timer due at +7m did not fire during Advance(7m)")
	}
	if _, ok := fired(t10); ok {
		t.Error("timer due at +10m fired early during Advance(7m)")
	}
	v.Advance(3 * time.Minute)
	if _, ok := fired(t10); !ok {
		t.Error("timer due at +10m did not fire once the clock reached +10m")
	}
}

func TestVirtualTimerFiresOnce(t *testing.T) {
	t.Parallel()
	v := NewVirtual(epoch)
	timer := v.NewTimer(time.Minute)
	v.Advance(time.Hour)
	if _, ok := fired(timer); !ok {
		t.Fatal("timer did not fire")
	}
	v.Advance(time.Hour)
	if _, ok := fired(timer); ok {
		t.Fatal("timer fired a second time")
	}
}

func TestVirtualTimerStop(t *testing.T) {
	t.Parallel()
	t.Run("a pending timer is stopped and never fires", func(t *testing.T) {
		t.Parallel()
		v := NewVirtual(epoch)
		timer := v.NewTimer(time.Minute)
		if !timer.Stop() {
			t.Fatal("Stop returned false for a pending timer")
		}
		if timer.Stop() {
			t.Fatal("Stop returned true for an already stopped timer")
		}
		v.Advance(time.Minute)
		if _, ok := fired(timer); ok {
			t.Fatal("stopped timer fired")
		}
	})
	t.Run("a fired timer cannot be stopped", func(t *testing.T) {
		t.Parallel()
		v := NewVirtual(epoch)
		timer := v.NewTimer(time.Minute)
		v.Advance(time.Minute)
		if timer.Stop() {
			t.Fatal("Stop returned true for a timer that already fired")
		}
	})
}
