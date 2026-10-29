package app_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

var (
	worldStart  = time.Unix(0, birthNS).UTC()
	sliceEnd    = worldStart.Add(60 * time.Second)
	wallAtStart = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
)

type reasoningProbe struct {
	mu       sync.Mutex
	decision *sources.Decision
	seen     []time.Time
}

func (p *reasoningProbe) check(context.Context) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seen = append(p.seen, p.decision.Now())
	return false, nil
}

func (p *reasoningProbe) readAt() []time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]time.Time(nil), p.seen...)
}

func worldHost(t *testing.T) (runningHost, *sources.Decision, *reasoningProbe) {
	t.Helper()
	decision := sources.NewDecision(sources.NewVirtual(wallAtStart))
	probe := &reasoningProbe{decision: decision}
	host := pumpHost(t, app.Config{World: decision, ReasoningInFlight: probe.check}, nil)
	return host, decision, probe
}

func wallNode(t *testing.T, host runningHost) *plantwirefake.Node {
	t.Helper()
	node := host.node(t, "s-1")
	birth := plantwirefake.Fixture(t, ".", "NBIRTH.json")
	birth["node_metrics"] = map[string]any{"clock_mode": "wall", "run_state": "running"}
	node.Send(birth)
	node.Expect("STATE")
	return node
}

func TestHostBindsTheDecisionClockToThePacedWorldAtItsFirstDeviceBirth(t *testing.T) {
	t.Parallel()
	host, decision, _ := worldHost(t)
	node := bornNode(t, host, "s-1")
	if decision.FollowsWorld() {
		t.Fatal("the decision clock followed the world at the node birth, before any world time")
	}
	node.Send(plantwirefake.Fixture(t, ".", "DBIRTH.json"))
	host.event(t)
	if got := decision.Now(); !decision.FollowsWorld() || !got.Equal(worldStart) {
		t.Fatalf("decision clock = %v following=%v, want the world start %v", got, decision.FollowsWorld(), worldStart)
	}
}

func TestHostMovesTheDecisionClockToTheBarrierWorldTimeBeforeTheReasoningCheck(t *testing.T) {
	t.Parallel()
	host, decision, probe := worldHost(t)
	node := bornNode(t, host, "s-1")
	node.Send(plantwirefake.Fixture(t, ".", "DBIRTH.json"))
	node.Send(pumpData(firstBoot, birthNS+60*second, "good", false))
	barrier(node)
	if !barrierAnswerWithin(node, 5*time.Second) {
		t.Fatal("the host never answered the barrier")
	}
	if got := decision.Now(); !got.Equal(sliceEnd) {
		t.Fatalf("decision clock after the barrier = %v, want the slice end %v", got, sliceEnd)
	}
	for _, at := range probe.readAt() {
		if !at.Equal(sliceEnd) {
			t.Errorf("the reasoning check ran with the decision clock at %v, want the slice end %v", at, sliceEnd)
		}
	}
	if len(probe.readAt()) == 0 {
		t.Fatal("the reasoning check never ran")
	}
}

func TestHostNeverMovesTheDecisionClockBackWhenAWorldTimeIsOlder(t *testing.T) {
	t.Parallel()
	host, decision, _ := worldHost(t)
	node := bornNode(t, host, "s-1")
	node.Send(plantwirefake.Fixture(t, ".", "DBIRTH.json"))
	node.Send(pumpData(firstBoot, birthNS+60*second, "good", false))
	barrier(node)
	barrierAnswerWithin(node, 5*time.Second)
	node.Send(map[string]any{"type": "NSYNC", "lane": "control", "through_seq": 0, "world_time_ns": birthNS + 10*second})
	if !barrierAnswerWithin(node, 5*time.Second) {
		t.Fatal("the host never answered the second barrier")
	}
	if got := decision.Now(); !got.Equal(sliceEnd) {
		t.Fatalf("decision clock after an older barrier = %v, want it kept at %v", got, sliceEnd)
	}
}

func TestHostLeavesTheDecisionClockOnTheWallForAWallPacedWorld(t *testing.T) {
	t.Parallel()
	host, decision, _ := worldHost(t)
	node := wallNode(t, host)
	node.Send(plantwirefake.Fixture(t, ".", "DBIRTH.json"))
	node.Send(pumpData(firstBoot, birthNS+60*second, "good", false))
	barrier(node)
	if !barrierAnswerWithin(node, 5*time.Second) {
		t.Fatal("the host never answered the barrier")
	}
	if got := decision.Now(); decision.FollowsWorld() || !got.Equal(wallAtStart) {
		t.Fatalf("decision clock = %v following=%v, want the wall time %v unfollowed", got, decision.FollowsWorld(), wallAtStart)
	}
}
