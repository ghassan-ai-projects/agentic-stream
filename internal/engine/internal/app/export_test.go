package app

import (
	"context"
)

// Run drains one partition under the run lock; production runs only RunGlobal.
func (s *Service) Run(ctx context.Context, partitionID int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.run(ctx, partitionID, nil)
}

// RunDueTimers fires one partition's due timers under the run lock.
func (s *Service) RunDueTimers(ctx context.Context, partitionID int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runDueTimers(ctx, partitionID)
}
