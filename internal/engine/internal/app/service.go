package app

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/engine/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/eventlog"
	"github.com/ghassan-ai-projects/agentic-stream/internal/operators"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

// Config supplies the fully configured store, the event log and compiled spec,
// the resolved tenant, and whether Situation versions reach cognition.
type Config struct {
	Store     store.Store
	Log       *eventlog.EventLog
	Clock     sources.Clock
	Spec      *spec.CompiledSpec
	TenantID  string
	Cognition bool
}

// Service processes events for one tenant deterministically. A single mutex
// serializes its runs.
type Service struct {
	mu           sync.Mutex
	store        store.Store
	log          *eventlog.EventLog
	clock        sources.Clock
	spec         *spec.CompiledSpec
	tenantID     string
	deploymentID string

	opRuntime *operators.OperatorRuntime
	sitEngine *situations.Engine
	cogEngine *cognition.Service
}

// New validates the configuration, records the deployment, configures schema
// validation, builds the planes and restores durable Situations.
func New(ctx context.Context, cfg Config) (*Service, error) {
	if !cfg.Store.Configured() || cfg.Log == nil || cfg.Spec == nil {
		return nil, errors.New("engine requires a database, runtime owner check, event log and compiled spec")
	}
	if err := cfg.Store.SaveDeployment(ctx, cfg.Spec); err != nil {
		return nil, err //nolint:wrapcheck // The store names the failed deployment write.
	}
	if domain.RequiresSchemaValidation(cfg.Spec.Inputs) {
		cfg.Log.RequireSchemaValidation()
	}
	service := &Service{store: cfg.Store, log: cfg.Log, clock: sources.OrPhysical(cfg.Clock), spec: cfg.Spec, tenantID: cfg.TenantID, deploymentID: cfg.Spec.Digest}
	if err := service.buildPlanes(ctx, cfg.Cognition); err != nil {
		return nil, err
	}
	return service, nil
}

// buildPlanes creates the operator runtime and situation engine over one
// deterministic ID sequence, restores durable Situations, then attaches
// cognition.
func (s *Service) buildPlanes(ctx context.Context, cognitionEnabled bool) error {
	idGen := sources.Deterministic()
	var err error
	if s.opRuntime, err = operators.NewOperatorRuntime(s.spec.Digest, s.spec, idGen); err != nil {
		return fmt.Errorf("operator runtime: %w", err)
	}
	if s.sitEngine, err = situations.NewEngine(s.spec.Digest, s.tenantID, 0, s.spec, idGen); err != nil {
		return fmt.Errorf("situation engine: %w", err)
	}
	if err := s.restoreSituations(ctx); err != nil {
		return fmt.Errorf("restore situations: %w", err)
	}
	s.cogEngine, err = s.newCognition(idGen, cognitionEnabled)
	return err
}

func (s *Service) newCognition(idGen sources.Generator, enabled bool) (*cognition.Service, error) {
	if !enabled {
		return nil, nil
	}
	engine, err := cognition.New(cognition.Config{DeploymentID: s.spec.Digest, TenantID: s.tenantID, Spec: s.spec, IDGen: idGen, Clock: s.clock})
	if err != nil {
		return nil, fmt.Errorf("cognition engine: %w", err)
	}
	return engine, nil
}

// restoreSituations rebuilds the in-memory Situations from storage.
func (s *Service) restoreSituations(ctx context.Context) error {
	return s.store.EachCurrentSituation(ctx, func(stored domain.StoredSituation) error {
		situation, err := stored.Restore(s.tenantID, s.deploymentID)
		if err != nil {
			return err //nolint:wrapcheck // The domain rule names the failed Situation.
		}
		if err := s.sitEngine.Restore(situation); err != nil {
			return fmt.Errorf("restore situation %s: %w", situation.SituationID, err)
		}
		return nil
	})
}

// restoreAfterRollback discards in-memory Situations a rolled-back transaction
// may have advanced and rebuilds them from storage.
func (s *Service) restoreAfterRollback(ctx context.Context) error {
	s.sitEngine.Reset()
	return s.restoreSituations(ctx)
}
