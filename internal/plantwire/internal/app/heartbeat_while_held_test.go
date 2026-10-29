package app_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

const heldIntervals = 4

func heldBeats(t *testing.T, host runningHost, node *plantwirefake.Node) []map[string]any {
	t.Helper()
	var beats []map[string]any
	for range heldIntervals {
		host.clock.Advance(domain.HeartbeatInterval)
		beats = append(beats, node.Expect("STATE"))
	}
	return beats
}

func releasedBarrier(t *testing.T, host runningHost, node *plantwirefake.Node) []map[string]any {
	t.Helper()
	var records []map[string]any
	for range 200 {
		host.clock.Advance(reasoningPoll)
		record, ok := node.Next(20 * time.Millisecond)
		if !ok {
			continue
		}
		records = append(records, record)
		if record["type"] == domain.TypeHostSync {
			return records
		}
	}
	t.Fatal("the host never answered the barrier after reasoning ended")
	return nil
}

func assertIncreasingSeq(t *testing.T, records []map[string]any, last int64) {
	t.Helper()
	for _, record := range records {
		seq := number(t, record["seq"])
		if seq <= last {
			t.Errorf("%v seq = %d after %d, want strictly increasing", record["type"], seq, last)
		}
		last = seq
	}
}

func TestHostKeepsItsHeartbeatWhileABarrierAnswerIsHeldAndNumbersTheAnswerAtRelease(t *testing.T) {
	t.Parallel()
	var busy atomic.Bool
	busy.Store(true)
	host, node := reasoningHost(t, func(context.Context) (bool, error) { return busy.Load(), nil })
	beats := heldBeats(t, host, node)
	for i, beat := range beats {
		if number(t, beat["heartbeat"]) != int64(i+1) || beat["online"] != true {
			t.Errorf("heartbeat %d during the hold = %v", i+1, beat)
		}
	}
	busy.Store(false)
	released := releasedBarrier(t, host, node)
	assertIncreasingSeq(t, append(beats, released...), 0)
	if answer := released[len(released)-1]; number(t, answer["seq"]) <= number(t, beats[len(beats)-1]["seq"]) {
		t.Errorf("the held HSYNC seq = %v, want it after the last heartbeat %v", answer["seq"], beats[len(beats)-1]["seq"])
	}
}
