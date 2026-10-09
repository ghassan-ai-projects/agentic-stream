package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	runtimecontrol "github.com/ghassan-ai-projects/agentic-stream/internal/control"
	"github.com/ghassan-ai-projects/agentic-stream/internal/evidence"
	domain "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/domain"
	store "github.com/ghassan-ai-projects/agentic-stream/internal/runtime/internal/store"
	"github.com/ghassan-ai-projects/agentic-stream/internal/sources"
)

type RecoveryReport = domain.RecoveryReport

type Service struct {
	owner  *runtimecontrol.RuntimeOwner
	ledger *evidence.Service
	epoch  string
	now    func() time.Time
	logger *slog.Logger

	mu       sync.RWMutex
	ready    bool
	readyErr error
	stop     context.CancelFunc
	done     chan struct{}
}

func NewService(owner *runtimecontrol.RuntimeOwner, ledger *evidence.Service, epoch string) (*Service, error) {
	if owner == nil || ledger == nil || epoch == "" {
		return nil, fmt.Errorf("runtime service is not configured")
	}
	return &Service{owner: owner, ledger: ledger, epoch: epoch, now: sources.NowFunc(owner.Now), logger: slog.Default()}, nil
}

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
	s.logger.Info("runtime ready", "epoch", s.epoch, "owner_lease", s.owner.Lease)
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
	lastRenewed := s.now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.renewOrLoseLease(ctx, &lastRenewed) {
				return
			}
		}
	}
}

func (s *Service) renewOrLoseLease(ctx context.Context, lastRenewed *time.Time) bool {
	err := s.maintainRuntimeLease(ctx)
	if err == nil {
		*lastRenewed = s.now()
		return true
	}
	if !domain.LeaseLapsed(*lastRenewed, s.now(), s.owner.Lease) {
		s.logger.Warn("runtime owner lease renewal failed; retrying within the lease", "epoch", s.epoch, "error", err)
		return true
	}
	s.logger.Error("runtime owner lease lost; runtime not ready", "epoch", s.epoch, "error", err)
	s.setNotReady(err)
	return false
}

func (s *Service) maintainRuntimeLease(ctx context.Context) error {
	if err := s.owner.Renew(ctx, s.epoch); err != nil {
		return fmt.Errorf("runtime owner heartbeat: %w", err)
	}
	if err := s.ledger.ReclaimExpired(ctx, s.now().UTC()); err != nil {
		return fmt.Errorf("evidence lease reclamation: %w", err)
	}
	return nil
}

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

func (s *Service) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.stopHeartbeat()
	if err := s.releaseOwner(ctx); err != nil {
		s.logger.Error("runtime owner lease not released", "epoch", s.epoch, "error", err)
		return err
	}
	s.logger.Info("runtime stopped; owner lease released", "epoch", s.epoch)
	return nil
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

func (s *Service) stopHeartbeat() {
	s.mu.Lock()
	stop, done := s.stop, s.done
	s.ready = false
	s.stop = nil
	s.mu.Unlock()
	if stop != nil {
		stop()
		<-done
	}
}
