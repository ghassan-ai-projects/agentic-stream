package app

import (
	"context"
)

// RunDueTimers fires one partition's due timers under the run lock.
func (s *Service) RunDueTimers(ctx context.Context, partitionID int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runDueTimers(ctx, partitionID)
}
