package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/domain"
	"github.com/ghassan-ai-projects/agentic-stream/internal/authority/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/clock"
)

// Fences are the runtime checks ordinary admission runs inside each admitted
// unit of work.
type Fences struct {
	RuntimeOwner store.Fence // the singleton runtime lease is held by the epoch
	EpochControl store.Fence // the epoch is neither draining nor killed
}

// Config wires the use cases for one runtime owner instance.
type Config struct {
	Store         *store.Store
	Fences        Fences
	Outcomes      store.OutcomeLedger // counts bound commands whose outcome is unresolved
	OwnerInstance string
	ClaimLease    time.Duration // how long a target claim stays live without renewal
	Clock         clock.Clock   // read once per operation
}

// Service runs the device-authority use cases for one runtime owner instance.
type Service struct {
	store         *store.Store
	fences        Fences
	outcomes      store.OutcomeLedger
	ownerInstance string
	claimLease    time.Duration
	clock         clock.Clock
}

// New returns the use cases configured by cfg.
func New(cfg Config) *Service {
	return &Service{
		store: cfg.Store, fences: cfg.Fences, outcomes: cfg.Outcomes,
		ownerInstance: cfg.OwnerInstance, claimLease: cfg.ClaimLease, clock: cfg.Clock,
	}
}

// OwnerInstance is the runtime owner instance every operation must name.
func (s *Service) OwnerInstance() string {
	return s.ownerInstance
}

// AssertRuntime verifies ordinary admission for epoch without touching device
// state.
func (s *Service) AssertRuntime(ctx context.Context, epoch string) error {
	if epoch == "" {
		return errors.New("owner epoch is required")
	}
	if err := s.inAdmittedTx(ctx, epoch, func(*store.Tx) error { return nil }); err != nil {
		return fmt.Errorf("assert runtime authority: %w", err)
	}
	return nil
}

func (s *Service) checkOwner(owner domain.Owner) error {
	if !owner.Complete() {
		return errors.New("owner epoch and owner instance are required")
	}
	if owner.Instance != s.ownerInstance {
		return fmt.Errorf("owner instance %q does not match runtime owner %q", owner.Instance, s.ownerInstance)
	}
	return nil
}

func (s *Service) checkClaim(claim domain.TargetClaim) error {
	if !claim.Complete() {
		return errors.New("target, device, boot, owner epoch and owner instance are required")
	}
	return s.checkOwner(claim.Owner)
}

// now is the operation's single clock read.
func (s *Service) now() time.Time {
	return s.clock.Now().UTC()
}
