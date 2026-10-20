package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority"
	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/transport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/plantwire/internal/wire"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

const (
	endHostStopping   = "the host is stopping"
	endConnectionLost = "the connection was lost"
	endNodeClosed     = "the node closed the connection"
	endWriteFailed    = "a host record could not be written"
	endPipelineBehind = "the pipeline fell behind and the ingest queue is full"
)

type timer interface {
	C() <-chan time.Time
	Stop() bool
}

type readResult struct {
	line []byte
	err  error
}

type session struct {
	conn         *transport.Conn
	conversation *domain.Conversation
	clock        sources.Clock
	beat         timer
	queue        chan<- contractsv1.Envelope
	lines        chan readResult
	calls        chan call
	authority    *authority.Service
	claimsMu     sync.Mutex
	claims       map[string]authority.TargetClaim
	mailboxes    map[string]chan<- domain.Settlement
	watchers     map[string]chan<- domain.Verification
	done         chan struct{}
}

func (h *Host) serve(ctx context.Context, conn *transport.Conn, queue chan<- contractsv1.Envelope) {
	s := &session{conn: conn, conversation: domain.NewConversation(h.admission, h.epoch, h.mapping), clock: h.clock,
		queue: queue, lines: make(chan readResult), calls: make(chan call), mailboxes: map[string]chan<- domain.Settlement{},
		watchers: map[string]chan<- domain.Verification{}, authority: h.authority, claims: map[string]authority.TargetClaim{},
		done: make(chan struct{})}
	h.goLive(s)
	defer h.goLive(nil)
	defer s.close()
	go s.read()
	end := s.run(ctx)
	tally := s.conversation.Tally()
	slog.InfoContext(ctx, "plant wire session ended", "reason", end, "events", tally.Events,
		"historical_records_not_ingested", tally.Historical, "records_dropped", tally.Dropped)
}

func (s *session) run(ctx context.Context) string {
	for {
		if end := s.step(ctx); end != "" {
			return end
		}
	}
}

func (s *session) step(ctx context.Context) string {
	select {
	case <-ctx.Done():
		_ = s.send(s.conversation.Goodbye())
		return endHostStopping
	case read := <-s.lines:
		return s.receive(ctx, read)
	case <-s.beats():
		return s.heartbeat()
	case do := <-s.calls:
		return do()
	}
}

func (s *session) receive(ctx context.Context, read readResult) string {
	if read.err != nil {
		return lostConnection(read.err)
	}
	reaction := s.conversation.Receive(s.arrivalOf(read.line))
	report(ctx, reaction)
	s.startBeating(reaction)
	s.deliver(reaction)
	if !enqueue(s.queue, reaction.Events) {
		return endPipelineBehind
	}
	if err := s.send(reaction.Replies); err != nil {
		slog.WarnContext(ctx, "plant wire host record not written", "error", err)
		return endWriteFailed
	}
	return reaction.End
}

func (s *session) startBeating(reaction domain.Reaction) {
	if reaction.Verdict != nil && reaction.Verdict.Accepted() {
		s.beat = s.clock.NewTimer(domain.HeartbeatInterval)
	}
}

func (s *session) arrivalOf(line []byte) domain.Arrival {
	arrival := arrivalOf(line)
	arrival.ReceivedAt = s.clock.Now()
	return arrival
}

func (s *session) heartbeat() string {
	s.beat = s.clock.NewTimer(domain.HeartbeatInterval)
	if err := s.send(s.conversation.Beat()); err != nil {
		return endWriteFailed
	}
	return ""
}

func (s *session) beats() <-chan time.Time {
	if s.beat == nil {
		return nil
	}
	return s.beat.C()
}

func (s *session) send(records []map[string]any) error {
	for _, record := range records {
		line, err := wire.Encode(record)
		if err != nil {
			return err
		}
		if err := s.conn.Write(line); err != nil {
			return err
		}
	}
	return nil
}

func (s *session) read() {
	for {
		line, err := s.conn.ReadLine()
		select {
		case s.lines <- readResult{line: line, err: err}:
		case <-s.done:
			return
		}
		if err != nil {
			return
		}
	}
}

func (s *session) close() {
	s.settle(s.conversation.Abandon())
	close(s.done)
	s.releaseClaims()
	if s.beat != nil {
		s.beat.Stop()
	}
	_ = s.conn.Close()
}

func arrivalOf(line []byte) domain.Arrival {
	in, err := wire.Decode(line)
	if err != nil {
		return domain.Arrival{Invalid: err}
	}
	return in.Arrival()
}

func lostConnection(err error) string {
	if errors.Is(err, io.EOF) {
		return endNodeClosed
	}
	return endConnectionLost
}
