package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
)

type Feed = func(context.Context, func(context.Context, contractsv1.Envelope) error) error

var ErrFeedWithoutSink = errors.New("an ingress feed needs a feed and a sink")

func (s *Service) ServeFeed(ctx context.Context, feed Feed, sink EnvelopeSink) error {
	if feed == nil || sink == nil {
		return ErrFeedWithoutSink
	}
	err := feed(ctx, func(ctx context.Context, env contractsv1.Envelope) error {
		return s.processFed(ctx, env, sink)
	})
	if err != nil {
		return fmt.Errorf("serve an ingress feed: %w", err)
	}
	return nil
}

func (s *Service) processFed(ctx context.Context, env contractsv1.Envelope, sink EnvelopeSink) error {
	verdict := s.admitFed(ctx, env)
	if !verdict.Rejected() {
		return sink(ctx, verdict.Envelope)
	}
	s.logger.WarnContext(ctx, "fed event rejected", "event_id", env.ID, "event_type", env.Type, "reason_code", verdict.Reason, "error", verdict.Cause)
	if err := s.quarantineEnvelope(ctx, verdict.Envelope, verdict.Reason, s.clk.Now()); err != nil {
		return fmt.Errorf("quarantine a fed event: %w", err)
	}
	return nil
}
