package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

type Service struct {
	store          store.Store
	clk            sources.Clock
	requireSchemas bool
}

func New(clk sources.Clock, st store.Store) *Service {
	clk = sources.OrPhysical(clk)
	return &Service{store: st, clk: clk}
}

func (s *Service) RequireSchemaValidation() *Service {
	s.requireSchemas = true
	return s
}

func (s *Service) CurrentPosition(ctx context.Context, tenantID string) (domain.LogPosition, error) {
	return s.store.CurrentPosition(ctx, tenantID) //nolint:wrapcheck // Store owns the query error context.
}

func (s *Service) ValidateEnvelope(ctx context.Context, env contractsv1.Envelope) error {
	if !s.requireSchemas {
		return nil
	}
	return s.store.Unit(ctx, func(u *store.Unit) error {
		return s.validateSchema(ctx, u, env)
	})
}

func (s *Service) validateSchema(ctx context.Context, u *store.Unit, env contractsv1.Envelope) error {
	schemaJSON, err := u.LoadEventSchemaJSON(ctx, env.Type, env.SchemaVersion)
	if err != nil {
		return err
	}
	schema, err := domain.DecodeEventSchema(schemaJSON)
	if err != nil {
		return err
	}
	return schema.CheckPayload(env.Data)
}

func (s *Service) ReadEntityEvents(ctx context.Context, window domain.EntityWindow, visit func(domain.EntityEvent) (bool, error)) error {
	return s.store.ReadEntityEvents(ctx, window, visit) //nolint:wrapcheck // Store owns the query error context.
}

func (s *Service) admit(ctx context.Context, u *store.Unit, tenantID string, env contractsv1.Envelope) error {
	if err := contractsv1.ValidateEnvelope(env, tenantID); err != nil {
		return fmt.Errorf("validate envelope: %w", err)
	}
	if !s.requireSchemas {
		return nil
	}
	return s.validateSchema(ctx, u, env)
}

func (s *Service) appendOne(ctx context.Context, u *store.Unit, tenantID string, env contractsv1.Envelope) (domain.LogPosition, error) {
	if _, err := contractsv1.ParseTraceContext(env.Traceparent, env.Tracestate); err != nil {
		return -1, fmt.Errorf("validate trace context: %w", err)
	}
	body, err := domain.EncodeEventBody(env.Data, env.Quality)
	if err != nil {
		return -1, err
	}
	createdAt := sources.FormatTime(s.clk.Now())
	return u.InsertEvent(ctx, tenantID, env, body, createdAt)
}
