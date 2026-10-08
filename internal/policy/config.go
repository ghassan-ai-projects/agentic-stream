package policy

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ghassan-ai-projects/agentic-stream/internal/policy/internal/app"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

// Config requires ownership, epoch and action-readiness checks. Consequential
// (R2) intents always need human approval.
type Config struct {
	PolicyVersion, OwnerEpoch   string
	IDGenerator                 sources.Generator
	RuntimeOwner, DecisionEpoch func(context.Context, *sql.Tx, string) error
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
	if c.RuntimeOwner == nil || c.DecisionEpoch == nil {
		return fmt.Errorf("policy ownership and decision epoch checks are required")
	}
	return nil
}
func applicationConfig(c Config, digest string) app.Config {
	generator := c.IDGenerator
	if generator == nil {
		generator = sources.Random()
	}
	cfg := app.Config{PolicyVersion: c.PolicyVersion, PolicyDigest: digest, OwnerEpoch: c.OwnerEpoch, IDGenerator: generator, Fences: app.Fences{RuntimeOwner: c.RuntimeOwner, DecisionEpoch: c.DecisionEpoch}}
	return cfg
}
