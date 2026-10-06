package app

import (
	"context"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/contractsv1"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/eventlog/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// Service holds the evidence-log use cases over one store and clock.
type Service struct {
	store          store.Store
	clk            sources.Clock
	requireSchemas bool
}

// New creates the use-case service over a store and clock. A nil clock falls
// back to the physical clock.
func New(clk sources.Clock, st store.Store) *Service {
	if clk == nil {
		clk = sources.Physical()
	}
	return &Service{store: st, clk: clk}
}

// RequireSchemaValidation makes every append and redrive validate envelopes
// against the durable event_schemas registry.
func (s *Service) RequireSchemaValidation() *Service {
	s.requireSchemas = true
	return s
}

// CurrentPosition returns the greatest durable log position for tenantID.
func (s *Service) CurrentPosition(ctx context.Context, tenantID string) (domain.LogPosition, error) {
	return s.store.CurrentPosition(ctx, tenantID) //nolint:wrapcheck // Store owns the query error context.
}

// ValidateEnvelope checks an envelope against the durable registered schema
// when schema validation is required.
func (s *Service) ValidateEnvelope(ctx context.Context, env contractsv1.Envelope) error {
	if !s.requireSchemas {
		return nil
	}
	return s.store.Unit(ctx, func(u *store.Unit) error {
		return s.validateSchema(ctx, u, env)
	})
}

// validateSchema loads the registered schema and checks the payload against
// it inside the open unit.
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

// ReadEntityEvents streams one entity window's events until visit wants no
// more.
func (s *Service) ReadEntityEvents(ctx context.Context, window domain.EntityWindow, visit func(domain.EntityEvent) (bool, error)) error {
	return s.store.ReadEntityEvents(ctx, window, visit) //nolint:wrapcheck // Store owns the query error context.
}

// RecordGap records a durable discontinuity. It never deletes evidence.
func (s *Service) RecordGap(ctx context.Context, gap domain.Gap) error {
	if err := gap.Valid(); err != nil {
		return err
	}
	return s.store.RecordGap(ctx, gap) //nolint:wrapcheck // Store owns the insert error context.
}

// admit validates the envelope contract and, when required, the registered
// event schema inside the open unit.
func (s *Service) admit(ctx context.Context, u *store.Unit, tenantID string, env contractsv1.Envelope) error {
	if err := contractsv1.ValidateEnvelope(env, tenantID); err != nil {
		return fmt.Errorf("validate envelope: %w", err)
	}
	if !s.requireSchemas {
		return nil
	}
	return s.validateSchema(ctx, u, env)
}

// appendOne validates trace context, encodes and inserts one envelope inside
// the open unit.
func (s *Service) appendOne(ctx context.Context, u *store.Unit, tenantID string, env contractsv1.Envelope) (domain.LogPosition, error) {
	if _, err := contractsv1.ParseTraceContext(env.Traceparent, env.Tracestate); err != nil {
		return -1, fmt.Errorf("validate trace context: %w", err)
	}
	body, err := domain.EncodeEventBody(env.Data, env.Quality)
	if err != nil {
		return -1, err
	}
	createdAt := s.clk.Now().UTC().Format(time.RFC3339Nano)
	return u.InsertEvent(ctx, tenantID, env, body, createdAt)
}
