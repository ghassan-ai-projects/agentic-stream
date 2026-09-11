// Package clock abstracts physical and virtual time for deterministic replay.
package clock

import (
	"cmp"
	"slices"
	"sync"
	"time"
)

// Clock provides time to the runtime. The physical clock is safe for concurrent
// use; the virtual clock serializes every method under its own mutex, so it is
// also safe for concurrent use.
type Clock interface {
	// Now returns the current clock time.
	Now() time.Time
	// NewTimer creates a timer that fires after d according to this clock.
	// Virtual timers are delivered through the virtual clock's Advance method.
	NewTimer(d time.Duration) Timer
}

// Quality identifies whether a clock is virtualized for replay or backed by
// wall time for live processing.
func Quality(c Clock) string {
	if _, ok := c.(*Virtual); ok {
		return "virtual"
	}
	return "physical"
}

// Timer is the minimal timer surface used by the runtime.
type Timer interface {
	// C returns the channel on which the timer fires.
	C() <-chan time.Time
	// Stop prevents the timer from firing. It returns true if the timer was
	// stopped before it fired.
	Stop() bool
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

// Virtual is a deterministic clock for tests and replay. It starts at start
// and advances only when Advance is called. During an Advance, every timer due
// at or before the new time fires in due-time order; timers with the same due
// time fire in the order they were scheduled.
type Virtual struct {
	mu     sync.Mutex
	now    time.Time
	timers []*virtualTimer
	seq    uint64
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
		seq:    v.seq,
	}
	v.seq++
	v.timers = append(v.timers, t)
	return t
}

// Advance moves the virtual clock forward by d and fires any due timers. Timers
// fire in due-time order, ties broken by scheduling order, so every timer due
// at the new time fires during this call regardless of scheduling sequence.
func (v *Virtual) Advance(d time.Duration) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.now = v.now.Add(d)
	// Sort by (due, seq) once: firing only pops from the front and never adds
	// timers, so the remainder stays ordered for the rest of this call.
	slices.SortStableFunc(v.timers, func(a, b *virtualTimer) int {
		if c := a.due.Compare(b.due); c != 0 {
			return c
		}
		return cmp.Compare(a.seq, b.seq)
	})
	for len(v.timers) > 0 {
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

type virtualTimer struct {
	clock  *Virtual
	due    time.Time
	active bool
	seq    uint64
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
