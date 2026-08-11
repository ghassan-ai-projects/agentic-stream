// Package clock abstracts physical and virtual time for deterministic replay.
package clock

import (
	"sync"
	"time"
)

// Clock provides time to the runtime. Implementations must be safe for the
// concurrency model of their consumer; the virtual clock is not safe for
// concurrent use without external synchronization.
type Clock interface {
	// Now returns the current clock time.
	Now() time.Time
	// NewTimer creates a timer that fires after d according to this clock.
	// Virtual timers are delivered through the virtual clock's Advance method.
	NewTimer(d time.Duration) Timer
}

// Timer is the minimal timer surface used by the runtime.
type Timer interface {
	// C returns the channel on which the timer fires.
	C() <-chan time.Time
	// Stop prevents the timer from firing. It returns true if the timer was
	// stopped before it fired.
	Stop() bool
	// Reset changes the timer to fire after d from now on the same clock.
	Reset(d time.Duration) bool
}

// Physical returns a clock backed by the operating system.
func Physical() Clock {
	return physicalClock{}
}

type physicalClock struct{}

func (physicalClock) Now() time.Time { return time.Now().UTC() }

func (physicalClock) NewTimer(d time.Duration) Timer {
	return physicalTimer{time.NewTimer(d)}
}

type physicalTimer struct {
	t *time.Timer
}

func (t physicalTimer) C() <-chan time.Time { return t.t.C }

func (t physicalTimer) Stop() bool { return t.t.Stop() }

func (t physicalTimer) Reset(d time.Duration) bool { return t.t.Reset(d) }

// Virtual is a deterministic clock for tests and replay. It starts at start
// and advances only when Advance is called. Timers fire during Advance in the
// order they were scheduled.
type Virtual struct {
	mu       sync.Mutex
	now      time.Time
	timers   []*virtualTimer
	modified bool
}

// NewVirtual creates a virtual clock with the given start time.
func NewVirtual(start time.Time) *Virtual {
	return &Virtual{now: start.UTC()}
}

// Now returns the current virtual time.
func (v *Virtual) Now() time.Time {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.now
}

// NewTimer creates a virtual timer that fires after d.
func (v *Virtual) NewTimer(d time.Duration) Timer {
	v.mu.Lock()
	defer v.mu.Unlock()

	t := &virtualTimer{
		clock:  v,
		due:    v.now.Add(d),
		c:      make(chan time.Time, 1),
		active: true,
	}
	v.timers = append(v.timers, t)
	v.modified = true
	return t
}

// Advance moves the virtual clock forward by d and fires any due timers.
func (v *Virtual) Advance(d time.Duration) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.now = v.now.Add(d)
	for {
		v.sortTimers()
		if len(v.timers) == 0 {
			break
		}
		next := v.timers[0]
		if next.due.After(v.now) {
			break
		}
		v.timers = v.timers[1:]
		if next.active {
			next.active = false
			next.c <- next.due
		}
	}
}

func (v *Virtual) sortTimers() {
	if !v.modified {
		return
	}
	v.modified = false
	// Simple insertion-style sort: find earliest due timer and move it to front.
	for i := 1; i < len(v.timers); i++ {
		if v.timers[i].due.Before(v.timers[0].due) {
			v.timers[0], v.timers[i] = v.timers[i], v.timers[0]
		}
	}
}

type virtualTimer struct {
	clock  *Virtual
	due    time.Time
	active bool
	c      chan time.Time
}

func (t *virtualTimer) C() <-chan time.Time { return t.c }

func (t *virtualTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	if !t.active {
		return false
	}
	t.active = false
	return true
}

func (t *virtualTimer) Reset(d time.Duration) bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	wasActive := t.active
	t.active = true
	t.due = t.clock.now.Add(d)
	t.clock.modified = true
	return wasActive
}
