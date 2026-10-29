package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/ghassan-ai-projects/agentic-stream/internal/draftport"
	"github.com/ghassan-ai-projects/agentic-stream/internal/executor/remote/internal/domain"
	runtimev1 "github.com/ghassan-ai-projects/agentic-stream/proto/agenticstream/runtime/v1"
)

type Drafter struct {
	client  runtimev1.EpisodeWorkerClient
	profile domain.Profile
}

func NewDrafter(client runtimev1.EpisodeWorkerClient, name, runtimeInstance string) *Drafter {
	return &Drafter{client: client, profile: domain.Profile{Name: name, RuntimeInstance: runtimeInstance}}
}

func (d *Drafter) Draft(ctx context.Context, request draftport.Request) (draftport.Reply, error) {
	if err := d.negotiate(ctx); err != nil {
		return draftport.Reply{}, err
	}
	wire, err := domain.DraftWireRequest(request)
	if err != nil {
		return draftport.Reply{}, err
	}
	stream, err := d.client.DraftPlaybookRule(ctx, wire)
	if err != nil {
		return draftport.Reply{}, fmt.Errorf("start draft %s: %w", request.DraftID, err)
	}
	return readDraft(stream)
}

func (d *Drafter) negotiate(ctx context.Context) error {
	handshake, err := d.client.Handshake(ctx, d.profile.HandshakeRequest())
	if err != nil {
		return fmt.Errorf("handshake for a draft: %w", err)
	}
	if !slices.Contains(handshake.GetSupportedFeatures(), draftport.Feature) {
		return draftport.ErrUnsupported
	}
	return nil
}

func readDraft(stream runtimev1.EpisodeWorker_DraftPlaybookRuleClient) (draftport.Reply, error) {
	var reading domain.DraftReading
	for {
		event, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return reading.Result()
		}
		if err != nil {
			return reading.Reply, fmt.Errorf("read the draft stream: %w", err)
		}
		if err := reading.Read(event); err != nil {
			return reading.Reply, err
		}
	}
}
