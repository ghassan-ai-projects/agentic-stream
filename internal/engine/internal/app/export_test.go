package app

import (
	"context"
)

func (s *Service) RunDueTimers(ctx context.Context, partitionID int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runDueTimers(ctx, partitionID, s.clock.Now().UTC())
}
