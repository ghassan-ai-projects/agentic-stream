// Package policy exposes deterministic intent governance and human approval.
package policy

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/qualification"
)

// Config requires ownership, epoch and action-readiness checks. Calibration is
// optional: without an exact artifact, consequential intents need human approval.
type Config struct {
	PolicyVersion, OwnerEpoch   string
	IDGenerator                 ids.Generator
	RuntimeOwner, DecisionEpoch func(context.Context, *sql.Tx, string) error
	Interlock                   interlock.Reader
	Calibration                 *qualification.CalibrationStore
}

// Service is the public facade over policy use cases.
type Service struct{ app *app.Service }

// New validates configuration before constructing a policy service.
func New(c Config) (*Service, error) {
	if err := validateConfig(c); err != nil {
		return nil, err
	}
	digest, err := DigestForVersion(c.PolicyVersion)
	if err != nil {
		return nil, fmt.Errorf("construct policy service: %w", err)
	}
	return &Service{app: app.New(applicationConfig(c, digest))}, nil
}
func validateConfig(c Config) error {
	if c.RuntimeOwner == nil || c.DecisionEpoch == nil || c.Interlock == nil {
		return fmt.Errorf("policy ownership, decision epoch and interlock checks are required")
	}
	return nil
}
func applicationConfig(c Config, digest string) app.Config {
	generator := c.IDGenerator
	if generator == nil {
		generator = ids.Random()
	}
	cfg := app.Config{PolicyVersion: c.PolicyVersion, PolicyDigest: digest, OwnerEpoch: c.OwnerEpoch, IDGenerator: generator, Fences: app.Fences{RuntimeOwner: c.RuntimeOwner, DecisionEpoch: c.DecisionEpoch}, Interlock: c.Interlock}
	if c.Calibration != nil {
		cfg.Calibration = c.Calibration.AssertCalibration
	}
	return cfg
}

// EvaluateIntent runs ordered gates and atomically records the outcome on tx.
func (s *Service) EvaluateIntent(ctx context.Context, tx *sql.Tx, r EvaluationRequest) (Result, error) {
	return s.app.EvaluateIntent(ctx, store.Join(tx), r)
}

// ResolveApproval records a human decision and re-evaluates before dispatch.
func (s *Service) ResolveApproval(ctx context.Context, tx *sql.Tx, r ApprovalResolution) (Result, error) {
	return s.app.ResolveApproval(ctx, store.Join(tx), r)
}
