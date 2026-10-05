package authority

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
	"github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// defaultClaimLease is how long a target claim stays live without renewal
// when Config.ClaimLease is zero.
const defaultClaimLease = time.Minute

// Config wires a Service. DB, Owner and Epochs are safety dependencies and
// are required; they must all use the same database.
type Config struct {
	DB         *storage.DB
	Owner      *control.RuntimeOwner
	Epochs     *control.EpochControl
	ClaimLease time.Duration
	Clock      clock.Clock
}

// Service is the device-authority module's only entry point.
type Service struct {
	db         *storage.DB
	owner      *control.RuntimeOwner
	epochs     *control.EpochControl
	claimLease time.Duration
	clock      clock.Clock
}

// New returns a Service, refusing a configuration that would silently skip
// a safety check.
func New(cfg Config) (*Service, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	service := &Service{db: cfg.DB, owner: cfg.Owner, epochs: cfg.Epochs, claimLease: cfg.ClaimLease, clock: cfg.Clock}
	if service.claimLease == 0 {
		service.claimLease = defaultClaimLease
	}
	if service.clock == nil {
		service.clock = clock.Physical()
	}
	return service, nil
}

func (cfg Config) validate() error {
	if cfg.DB == nil || cfg.Owner == nil || cfg.Epochs == nil {
		return errors.New("device authority requires a database, runtime owner and epoch control")
	}
	if cfg.Owner.DB != cfg.DB || cfg.Epochs.DB != cfg.DB {
		return errors.New("device authority, runtime owner and epoch control must share one database")
	}
	if cfg.Owner.InstanceID == "" {
		return errors.New("device authority requires a runtime owner instance")
	}
	if cfg.ClaimLease < 0 {
		return fmt.Errorf("device authority claim lease %s is negative", cfg.ClaimLease)
	}
	return nil
}

// OwnerInstance is the runtime owner instance every operation must name.
func (s *Service) OwnerInstance() string {
	return s.owner.InstanceID
}

// AssertRuntime verifies ordinary admission for epoch without touching device
// state.
func (s *Service) AssertRuntime(ctx context.Context, epoch string) error {
	if epoch == "" {
		return errors.New("owner epoch is required")
	}
	if err := s.withAdmittedTx(ctx, epoch, func(*sql.Tx) error { return nil }); err != nil {
		return fmt.Errorf("assert runtime authority: %w", err)
	}
	return nil
}

func (s *Service) checkOwner(owner Owner) error {
	if !owner.Complete() {
		return errors.New("owner epoch and owner instance are required")
	}
	if owner.Instance != s.owner.InstanceID {
		return fmt.Errorf("owner instance %q does not match runtime owner %q", owner.Instance, s.owner.InstanceID)
	}
	return nil
}

func (s *Service) checkClaim(claim TargetClaim) error {
	if !claim.Complete() {
		return errors.New("target, device, boot, owner epoch and owner instance are required")
	}
	return s.checkOwner(claim.Owner)
}
