package domain

import (
	"sync"
	"time"
)

type Decision struct {
	mu    sync.Mutex
	wall  Clock
	world *Virtual
}

func NewDecision(wall Clock) *Decision {
	return &Decision{wall: wall}
}

func (d *Decision) Now() time.Time {
	return d.current().Now()
}

func (d *Decision) NewTimer(after time.Duration) Timer {
	return d.current().NewTimer(after)
}

func (d *Decision) FollowWorld(at time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.world == nil {
		d.world = NewVirtual(at)
		return
	}
	d.world.AdvanceTo(at)
}

func (d *Decision) FollowsWorld() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.world != nil
}

func (d *Decision) current() Clock {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.world != nil {
		return d.world
	}
	return d.wall
}
