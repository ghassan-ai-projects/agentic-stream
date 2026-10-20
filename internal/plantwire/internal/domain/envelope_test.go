package domain_test

import (
	"errors"
	"math"
	"testing"

	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
)

func TestReceiverClassifiesEachSequenceNumberOnAnOrderedStream(t *testing.T) {
	t.Parallel()
	var receiver domain.Receiver
	steps := []struct {
		seq  uint64
		want domain.Order
	}{
		{0, domain.InOrder}, {0, domain.Duplicate}, {1, domain.InOrder}, {3, domain.Gap},
		{4, domain.InOrder}, {2, domain.Duplicate}, {math.MaxUint64, domain.Gap},
	}
	for i, step := range steps {
		if got := receiver.Accept(step.seq); got != step.want {
			t.Fatalf("step %d: Accept(%d) = %d, want %d", i, step.seq, got, step.want)
		}
	}
}

func TestEnvelopeIsWithinOnlyItsSessionAndNegotiatedVersion(t *testing.T) {
	t.Parallel()
	envelope := domain.Envelope{Type: "DDATA", ProtocolVersion: 1, SessionID: "s-1", Seq: 4}
	if err := envelope.Within("s-1", 1); err != nil {
		t.Fatalf("Within(own session) = %v, want nil", err)
	}
	for _, other := range []struct {
		session string
		version int64
	}{{"s-2", 1}, {"s-1", 2}} {
		if err := envelope.Within(other.session, other.version); !errors.Is(err, domain.ErrOutsideSession) {
			t.Errorf("Within(%q, %d) = %v, want ErrOutsideSession", other.session, other.version, err)
		}
	}
}
