package app_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

const reasoningPoll = 50 * time.Millisecond

func reasoningHost(t *testing.T, reasoning app.ReasoningInFlight) (runningHost, *plantwirefake.Node) {
	t.Helper()
	host := pumpHost(t, app.Config{ReasoningInFlight: reasoning}, nil)
	node := bornNode(t, host, "s-1")
	node.Send(plantwirefake.Fixture(t, ".", "DBIRTH.json"))
	node.Send(pumpData(firstBoot, birthNS+60*second, "good", false))
	barrier(node)
	return host, node
}

func answeredWhilePolling(host runningHost, node *plantwirefake.Node, polls int) bool {
	for range polls {
		host.clock.Advance(reasoningPoll)
		if barrierAnswerWithin(node, 20*time.Millisecond) {
			return true
		}
	}
	return false
}

func TestHostHoldsABarrierAnswerWhileReasoningIsInFlightAndReleasesItWhenItEnds(t *testing.T) {
	t.Parallel()
	var busy atomic.Bool
	busy.Store(true)
	host, node := reasoningHost(t, func(context.Context) (bool, error) { return busy.Load(), nil })
	if answeredWhilePolling(host, node, 20) {
		t.Fatal("the host answered the barrier while reasoning was in flight")
	}
	busy.Store(false)
	if !answeredWhilePolling(host, node, 200) {
		t.Fatal("the host never answered the barrier after reasoning ended")
	}
}

func TestHostAnswersABarrierAtOnceWhenNoReasoningIsInFlight(t *testing.T) {
	t.Parallel()
	_, node := reasoningHost(t, func(context.Context) (bool, error) { return false, nil })
	if !barrierAnswerWithin(node, 5*time.Second) {
		t.Fatal("the host held the barrier although no reasoning was in flight")
	}
}

func TestHostAnswersABarrierWhenItCannotTellWhetherReasoningIsInFlight(t *testing.T) {
	t.Parallel()
	_, node := reasoningHost(t, func(context.Context) (bool, error) { return true, errors.New("database is locked") })
	if !barrierAnswerWithin(node, 5*time.Second) {
		t.Fatal("the host held the barrier because the in-flight check failed")
	}
}
