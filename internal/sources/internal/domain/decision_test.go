package domain

import (
	"testing"
	"time"
)

func TestDecisionClockReadsTheWallClockUntilAWorldTakesIt(t *testing.T) {
	t.Parallel()
	wall := NewVirtual(epoch)
	decision := NewDecision(wall)
	wall.Advance(time.Hour)
	if got, want := decision.Now(), epoch.Add(time.Hour); !got.Equal(want) || decision.FollowsWorld() {
		t.Fatalf("Now() = %v following=%v, want the wall time %v unfollowed", got, decision.FollowsWorld(), want)
	}
}

func TestDecisionClockStartsAtTheFirstWorldTimeAndIgnoresTheWall(t *testing.T) {
	t.Parallel()
	wall := NewVirtual(epoch.AddDate(0, 9, 0))
	decision := NewDecision(wall)
	decision.FollowWorld(epoch)
	wall.Advance(time.Hour)
	if got := decision.Now(); !got.Equal(epoch) || !decision.FollowsWorld() {
		t.Fatalf("Now() = %v following=%v, want the world start %v", got, decision.FollowsWorld(), epoch)
	}
}

func TestDecisionClockMovesOnlyForwardAndFiresDueTimersOnTheMove(t *testing.T) {
	t.Parallel()
	decision := NewDecision(NewVirtual(epoch.AddDate(1, 0, 0)))
	decision.FollowWorld(epoch)
	timer := decision.NewTimer(time.Minute)
	decision.FollowWorld(epoch.Add(30 * time.Second))
	if _, ok := fired(timer); ok {
		t.Fatal("timer fired before the world reached it")
	}
	decision.FollowWorld(epoch.Add(10 * time.Second))
	if got, want := decision.Now(), epoch.Add(30*time.Second); !got.Equal(want) {
		t.Fatalf("Now() after an earlier world time = %v, want %v", got, want)
	}
	decision.FollowWorld(epoch.Add(time.Minute))
	if at, ok := fired(timer); !ok || !at.Equal(epoch.Add(time.Minute)) {
		t.Fatalf("timer fired = %v at %v, want it to fire with its due time", ok, at)
	}
}

func TestDecisionClockQualityIsTheQualityOfTheClockItReads(t *testing.T) {
	t.Parallel()
	decision := NewDecision(otherClock{})
	if got := Quality(decision); got != "physical" {
		t.Fatalf("Quality before a world = %q, want physical", got)
	}
	decision.FollowWorld(epoch)
	if got := Quality(decision); got != "virtual" {
		t.Fatalf("Quality after a world = %q, want virtual", got)
	}
}

func TestVirtualClockAdvanceToNeverMovesBack(t *testing.T) {
	t.Parallel()
	v := NewVirtual(epoch)
	v.AdvanceTo(epoch.Add(time.Hour))
	v.AdvanceTo(epoch)
	if got, want := v.Now(), epoch.Add(time.Hour); !got.Equal(want) {
		t.Fatalf("Now() = %v, want %v", got, want)
	}
}
