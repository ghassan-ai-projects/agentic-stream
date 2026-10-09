package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
)

type recordingProcessor struct {
	tx      *sql.Tx
	version situations.Version
	err     error
}

func (p *recordingProcessor) Process(_ context.Context, tx *sql.Tx, version situations.Version) error {
	p.tx, p.version = tx, version
	return p.err
}

func TestProcessVersionHandsTheVersionAndTheOpenTransactionToTheProcessor(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	failure := errors.New("cognition refused")
	processor := &recordingProcessor{err: failure}
	err := s.WithTx(t.Context(), func(tx *Tx) error { return tx.ProcessVersion(t.Context(), processor, publishedVersion(3)) })
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v, want the processor's error", err)
	}
	if processor.tx == nil || processor.version.Version != 3 || processor.version.SituationID != "sit-1" {
		t.Fatalf("processor saw tx %v version %+v, want the unit's transaction and version 3", processor.tx, processor.version)
	}
}
