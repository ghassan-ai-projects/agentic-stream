package app

import (
	"context"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

const defaultQueueSize = 4096

type Emit = func(context.Context, contractsv1.Envelope) error

func feedPipeline(ctx context.Context, queue <-chan contractsv1.Envelope, emit Emit, stop context.CancelCauseFunc) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case event := <-queue:
			if err := emit(ctx, event); err != nil {
				stop(err)
				return err
			}
		}
	}
}

func enqueue(queue chan<- contractsv1.Envelope, events []contractsv1.Envelope) bool {
	for _, event := range events {
		select {
		case queue <- event:
		default:
			return false
		}
	}
	return true
}
