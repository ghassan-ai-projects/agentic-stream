package policy

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/ids"
	"github.com/ghassan-ai-projects/agentic-stream/internal/interlock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/app"
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
