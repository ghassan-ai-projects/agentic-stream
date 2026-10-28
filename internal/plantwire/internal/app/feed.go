package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

const (
	defaultQueueSize = 4096
	backlogShare     = 4
)

type Emit = func(context.Context, contractsv1.Envelope) error

type ReasoningInFlight = func(context.Context) (bool, error)

type ingestQueue struct {
	events  chan contractsv1.Envelope
	drained chan struct{}
}

func newIngestQueue(size int) *ingestQueue {
	return &ingestQueue{events: make(chan contractsv1.Envelope, size), drained: make(chan struct{}, 1)}
}

func (q *ingestQueue) feed(ctx context.Context, emit Emit, stop context.CancelCauseFunc) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case event := <-q.events:
			if err := emit(ctx, event); err != nil {
				stop(err)
				return err
			}
			q.signalDrained()
		}
	}
}

func (q *ingestQueue) signalDrained() {
	if len(q.events) > 0 {
		return
	}
	select {
	case q.drained <- struct{}{}:
	default:
	}
}

func (q *ingestQueue) pending() bool {
	return len(q.events) > 0
}

func (q *ingestQueue) backedUp() bool {
	return len(q.events) >= max(cap(q.events)/backlogShare, 1)
}

func (q *ingestQueue) enqueue(events []contractsv1.Envelope) bool {
	for _, event := range events {
		select {
		case q.events <- event:
		default:
			return false
		}
	}
	return true
}
