package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
)

// RecoveryReport is the pure value returned after atomic durable recovery.
type RecoveryReport = domain.RecoveryReport

// Service owns the live runtime lease and readiness lifecycle. Ingestion and
// episode dispatch are enabled by later composition layers only after Service
// reports ready.
type Service struct {
	owner  *runtimecontrol.RuntimeOwner
	ledger *evidence.Service
	epoch  string

	mu       sync.RWMutex
	ready    bool
	readyErr error
	stop     context.CancelFunc
	done     chan struct{}
}

// NewService creates a runtime lifecycle around an already configured owner,
// ledger, and epoch.
func NewService(owner *runtimecontrol.RuntimeOwner, ledger *evidence.Service, epoch string) (*Service, error) {
	if owner == nil || ledger == nil || epoch == "" {
		return nil, fmt.Errorf("runtime service is not configured")
	}
	return &Service{owner: owner, ledger: ledger, epoch: epoch}, nil
}

// Start claims ownership, recovers durable state, and starts the heartbeat
// before marking the service ready.
func (s *Service) Start(ctx context.Context) (RecoveryReport, error) {
	if s == nil {
		return RecoveryReport{}, fmt.Errorf("runtime service is nil")
	}
	coordinator := &store.RecoveryCoordinator{Owner: s.owner, Ledger: s.ledger, Epoch: s.epoch, Costs: &runtimecontrol.CostLedger{}}
	report, err := coordinator.ClaimAndRecover(ctx)
	if err != nil {
		s.setNotReady(err)
		return RecoveryReport{}, err
	}
	s.startHeartbeat()
	return report, nil
}

func (s *Service) startHeartbeat() {
	heartbeatCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.stop = cancel
	s.done = make(chan struct{})
	s.ready = true
	s.readyErr = nil
	done := s.done
	s.mu.Unlock()
	go s.heartbeat(heartbeatCtx, done)
}

func (s *Service) heartbeat(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(domain.OwnerHeartbeatInterval(s.owner.Lease))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.maintainRuntimeLease(ctx); err != nil {
				s.setNotReady(err)
				return
			}
		}
	}
}

func (s *Service) maintainRuntimeLease(ctx context.Context) error {
	if err := s.owner.Renew(ctx, s.epoch); err != nil {
		return fmt.Errorf("runtime owner heartbeat: %w", err)
	}
	if err := s.ledger.ReclaimExpired(ctx, time.Now().UTC()); err != nil {
		return fmt.Errorf("evidence lease reclamation: %w", err)
	}
	return nil
}

// Ready reports whether ownership, recovery, and heartbeat health permit live
// work. It implements api.Readiness.
func (s *Service) Ready() error {
	if s == nil {
		return errors.New("runtime service is nil")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.ready {
		if s.readyErr != nil {
			return s.readyErr
		}
		return errors.New("runtime service is not ready")
	}
	return nil
}

// Close stops heartbeats, clears readiness, and releases the owner lease.
func (s *Service) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	stop, done := s.stop, s.done
	s.ready = false
	s.stop = nil
	s.mu.Unlock()
	if stop != nil {
		stop()
		<-done
	}
	return s.releaseOwner(ctx)
}

func (s *Service) releaseOwner(ctx context.Context) error {
	if err := s.owner.Release(ctx, s.epoch); err != nil {
		return fmt.Errorf("release runtime owner: %w", err)
	}
	return nil
}

func (s *Service) setNotReady(err error) {
	s.mu.Lock()
	s.ready = false
	s.readyErr = err
	s.mu.Unlock()
}
