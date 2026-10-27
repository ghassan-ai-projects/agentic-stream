package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/testsupport/plantwirefake"
)

func TestHostHoldsABarrierAnswerUntilTheIngestQueueHasDrained(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	slow := func(ctx context.Context, _ contractsv1.Envelope) error {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	}
	host := pumpHost(t, app.Config{QueueSize: 8}, slow)
	node := bornNode(t, host, "s-1")
	node.Send(plantwirefake.Fixture(t, ".", "DBIRTH.json"))
	node.Send(pumpData(firstBoot, birthNS+60*second, "good", false))
	barrier(node)
	if answered := barrierAnswerWithin(node, 300*time.Millisecond); answered {
		t.Fatal("the host answered the barrier while its ingest queue still held events")
	}
	close(release)
	if !barrierAnswerWithin(node, 5*time.Second) {
		t.Fatal("the host never answered the barrier after its ingest queue drained")
	}
}

func barrierAnswerWithin(node *plantwirefake.Node, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		record, ok := node.Next(time.Until(deadline))
		if !ok {
			return false
		}
		if record["type"] == "HSYNC" {
			return true
		}
	}
	return false
}
