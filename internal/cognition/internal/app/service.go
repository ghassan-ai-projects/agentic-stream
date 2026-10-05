package app

import (
	"context"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/cognition/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/situations"
	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

type Service struct {
	deploymentID, tenantID string
	spec                   *spec.CompiledSpec
	rules                  *domain.Rules
	scheduler              *scheduler
	clk                    clock.Clock
}

type scheduler struct {
	spec  *spec.CompiledSpec
	idGen ids.Generator
	clk   clock.Clock
}

type Config struct {
	DeploymentID, TenantID string
	Spec                   *spec.CompiledSpec
	IDGen                  ids.Generator
	Clock                  clock.Clock
}

func New(c Config) (*Service, error) {
	if c.Spec == nil || c.DeploymentID == "" || c.TenantID == "" {
		return nil, fmt.Errorf("cognition spec, deployment and tenant are required")
	}
	if c.Clock == nil {
		c.Clock = clock.Physical()
	}
	if c.IDGen == nil {
		c.IDGen = ids.Random()
	}
	rules, err := domain.NewRules(c.Spec)
	if err != nil {
		return nil, err
	}
	return &Service{deploymentID: c.DeploymentID, tenantID: c.TenantID, spec: c.Spec, rules: rules, clk: c.Clock, scheduler: &scheduler{spec: c.Spec, idGen: c.IDGen, clk: c.Clock}}, nil
}

// Process evaluates all triggers for a new Situation version and updates the
// durable scheduler queue. It runs inside the supplied transaction.
func (e *Service) Process(ctx context.Context, tx *store.Tx, v situations.Version) error {
	if !tx.Configured() {
		return fmt.Errorf("cognition caller transaction is required")
	}
	previous, err := e.lastReasonedVersion(ctx, tx, v.SituationID)
	if err != nil {
		return err
	}
	if err := e.evaluateTriggers(ctx, tx, v, previous); err != nil {
		return err
	}
	if _, err := e.admitReconsiderations(ctx, tx, v); err != nil {
		return fmt.Errorf("admit reconsideration: %w", err)
	}
	return tx.MarkVersionReasoned(ctx, v)
}

// lastReasonedVersion loads the version cognition last reasoned about, which
// is nil before the first one.
func (e *Service) lastReasonedVersion(ctx context.Context, tx *store.Tx, situationID string) (*situations.Version, error) {
	lastReasoned, err := tx.LoadLastReasonedVersion(ctx, situationID)
	if err != nil {
		return nil, fmt.Errorf("load last reasoned version: %w", err)
	}
	previous, err := tx.LoadVersion(ctx, situationID, lastReasoned)
	if err != nil {
		return nil, fmt.Errorf("load previous version: %w", err)
	}
	return previous, nil
}

func (e *Service) evaluateTriggers(ctx context.Context, tx *store.Tx, v situations.Version, previous *situations.Version) error {
	for _, tr := range e.spec.Cognition.Triggers {
		eval, err := e.rules.Evaluate(domain.EvaluationInput{Trigger: tr, Current: v, Previous: previous, Now: e.clk.Now().UTC(), DeploymentID: e.deploymentID})
		if err != nil {
			return fmt.Errorf("evaluate trigger %s: %w", tr.Name, err)
		}
		if err := e.scheduler.Admit(ctx, tx, eval, v, e.tenantID, e.deploymentID); err != nil {
			return fmt.Errorf("admit trigger %s: %w", tr.Name, err)
		}
	}
	return nil
}
