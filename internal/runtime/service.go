package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/costcontrol"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	"github.com/ghassan-ai-projects/agentic-stream/internal/storage"
)

// Service owns the live runtime lease and readiness lifecycle. Ingestion and
// episode dispatch are enabled by later composition layers only after Service
// reports ready.
type Service struct {
	owner  *storage.RuntimeOwner
	ledger *evidence.Ledger
	epoch  string

	mu       sync.RWMutex
	ready    bool
	readyErr error
	stop     context.CancelFunc
	done     chan struct{}
}

// NewService creates a runtime lifecycle around an already configured owner,
// ledger, and epoch.
func NewService(owner *storage.RuntimeOwner, ledger *evidence.Ledger, epoch string) (*Service, error) {
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
	coordinator := &RecoveryCoordinator{Owner: s.owner, Ledger: s.ledger, Epoch: s.epoch, Costs: &costcontrol.Controller{}}
	report, err := coordinator.ClaimAndRecover(ctx)
	if err != nil {
		s.setNotReady(err)
		return RecoveryReport{}, err
	}
	heartbeatCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.stop = cancel
	s.done = make(chan struct{})
	s.ready = true
	s.readyErr = nil
	done := s.done
	s.mu.Unlock()
	go s.heartbeat(heartbeatCtx, done)
	return report, nil
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
	return s.owner.Release(ctx, s.epoch)
}

func (s *Service) heartbeat(ctx context.Context, done chan struct{}) {
	defer close(done)
	interval := s.owner.Lease / 3
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.owner.Renew(ctx, s.epoch); err != nil {
				s.setNotReady(fmt.Errorf("runtime owner heartbeat: %w", err))
				return
			}
			if err := s.ledger.ReclaimExpired(ctx, time.Now().UTC()); err != nil {
				s.setNotReady(fmt.Errorf("evidence lease reclamation: %w", err))
				return
			}
		}
	}
}

func (s *Service) setNotReady(err error) {
	s.mu.Lock()
	s.ready = false
	s.readyErr = err
	s.mu.Unlock()
}
