package ingress

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
)

// appendBatchSize is how many envelopes a file connector appends per event
// log transaction.
const appendBatchSize = 100

// envelopeBatch buffers validated envelopes and appends them to the event log
// in batches, counting only the envelopes the log had not seen.
type envelopeBatch struct {
	log      *eventlog.EventLog
	tenantID string
	pending  []contractsv1.Envelope
	appended int
}

func (b *envelopeBatch) add(ctx context.Context, env contractsv1.Envelope) error {
	b.pending = append(b.pending, env)
	if len(b.pending) >= appendBatchSize {
		return b.flush(ctx)
	}
	return nil
}

func (b *envelopeBatch) flush(ctx context.Context) error {
	if len(b.pending) == 0 {
		return nil
	}
	positions, err := b.log.Append(ctx, b.tenantID, b.pending)
	if err != nil {
		return fmt.Errorf("append batch: %w", err)
	}
	for _, position := range positions {
		if position >= 0 {
			b.appended++
		}
	}
	b.pending = b.pending[:0]
	return nil
}
