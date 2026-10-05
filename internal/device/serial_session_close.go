package device

import (
	"context"
	"errors"
	"fmt"

	deviceauthority "github.com/ghassan-ai-projects/agentic-stream/internal/authority"
)

// Close releases the gateway link. It is safe to call more than once.
func (s *DeviceSession) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.closeSessionTransport()
}

func (s *DeviceSession) closeSessionTransport() error {
	releaseErr := s.releaseClaims()
	if s.transport == nil {
		if releaseErr != nil {
			return fmt.Errorf("release device target claims: %w", releaseErr)
		}
		return nil
	}
	if err := errors.Join(releaseErr, s.transport.Close()); err != nil {
		return fmt.Errorf("close device session: %w", err)
	}
	return nil
}

func (s *DeviceSession) releaseClaims() error {
	if s.authority == nil {
		return nil
	}
	var releaseErr error
	for target := range s.claimedTargets {
		err := s.authority.ReleaseClaim(context.Background(), s.targetClaim(target))
		if errors.Is(err, deviceauthority.ErrTargetClaimNotOwned) {
			continue
		}
		releaseErr = errors.Join(releaseErr, err)
	}
	// Close adds the operation context at the public boundary.
	return releaseErr //nolint:wrapcheck // Close wraps the joined release errors.
}
